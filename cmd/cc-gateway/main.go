package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"ccgateway/internal/ccevent"
	"ccgateway/internal/ccrun"
	"ccgateway/internal/gateway"
	"ccgateway/internal/mcpregistry"
	"ccgateway/internal/modelmap"
	"ccgateway/internal/plan"
	"ccgateway/internal/policy"
	"ccgateway/internal/probe"
	"ccgateway/internal/runlog"
	"ccgateway/internal/scheduler"
	"ccgateway/internal/session"
	"ccgateway/internal/settings"
	"ccgateway/internal/statepersist"
	"ccgateway/internal/todo"
	"ccgateway/internal/toolcatalog"
	"ccgateway/internal/upstream"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	routes, err := upstream.ParseRoutesFromEnv()
	if err != nil {
		log.Fatalf("invalid upstream route config: %v", err)
	}

	adapters, err := upstream.ParseAdaptersFromEnv()
	if err != nil {
		log.Fatalf("invalid upstream adapter config: %v", err)
	}
	defaultRouteFallback := []string{}
	if len(adapters) == 0 {
		primaryFail := os.Getenv("MOCK_PRIMARY_FAIL") == "true"
		adapters = []upstream.Adapter{
			upstream.NewMockAdapter("mock-primary", primaryFail),
			upstream.NewMockAdapter("mock-fallback", false),
		}
		defaultRouteFallback = []string{"mock-primary", "mock-fallback"}
	} else {
		defaultRouteFallback = adapterNames(adapters)
	}
	selector, err := scheduler.NewFromEnv(defaultRouteFallback)
	if err != nil {
		log.Fatalf("invalid scheduler config: %v", err)
	}
	judge, err := upstream.NewJudgeFromEnv(adapters, defaultRouteFallback)
	if err != nil {
		log.Fatalf("invalid judge config: %v", err)
	}

	// Election: auto-intelligence evaluation + scheduler model election
	election := scheduler.NewElection(scheduler.ElectionConfig{
		Enabled:            upstream.ParseBoolEnv("ENABLE_TASK_DISPATCH", false),
		MinScoreDifference: 5,
	})
	election.SetOnChange(func(result scheduler.ElectionResult) {
		log.Printf("election: scheduler=%s (score=%.0f), workers=%d, reason=%s",
			result.SchedulerAdapter, result.SchedulerScore,
			len(result.Workers), result.Reason)
	})

	// Dispatcher: routes complex requests to scheduler, simple to workers
	dispatcher := upstream.NewDispatcher(upstream.DispatchConfig{
		Enabled: upstream.ParseBoolEnv("ENABLE_TASK_DISPATCH", false),
	}, election)

	svc := upstream.NewRouterService(upstream.RouterConfig{
		Routes:              routes,
		DefaultRoute:        upstream.ParseListEnv("UPSTREAM_DEFAULT_ROUTE", defaultRouteFallback),
		Timeout:             upstream.ParseDurationEnv("UPSTREAM_TIMEOUT", 30*time.Second),
		Retries:             upstream.ParseIntEnv("UPSTREAM_RETRIES", 1),
		ReflectionPasses:    upstream.ParseIntEnv("REFLECTION_PASSES", 1),
		ParallelCandidates:  upstream.ParseIntEnv("PARALLEL_CANDIDATES", 1),
		EnableResponseJudge: upstream.ParseBoolEnv("ENABLE_RESPONSE_JUDGE", false),
		Judge:               judge,
		Selector:            selector,
		Dispatcher:          dispatcher,
	}, adapters)
	mapper, err := modelmap.NewFromEnv()
	if err != nil {
		log.Fatalf("invalid model mapping config: %v", err)
	}
	settingsStore, err := settings.NewFromEnv()
	if err != nil {
		log.Fatalf("invalid runtime settings: %v", err)
	}
	tools, err := toolcatalog.NewFromEnv()
	if err != nil {
		log.Fatalf("invalid tool catalog: %v", err)
	}
	logPath := os.Getenv("RUN_LOG_PATH")
	if logPath == "" {
		logPath = "logs/run-events.log"
	}
	runLogger, err := runlog.NewFileLogger(logPath)
	if err != nil {
		log.Fatalf("failed to init run logger: %v", err)
	}
	probeCfg, err := probe.ConfigFromEnv()
	if err != nil {
		log.Fatalf("invalid probe config: %v", err)
	}
	probeRunner := probe.NewRunner(probeCfg, adapters, selector)
	sessionStore := session.NewStore()
	runStore := ccrun.NewStore()
	todoStore := todo.NewStore()
	planStore := plan.NewStore()
	eventStore := ccevent.NewStore()
	persistDir := strings.TrimSpace(os.Getenv("STATE_PERSIST_DIR"))
	if persistDir != "" {
		backend, err := statepersist.NewFileBackend(persistDir)
		if err != nil {
			log.Fatalf("invalid state persistence backend: %v", err)
		}
		persistManager := statepersist.NewManager(backend, runStore, planStore, todoStore)
		persistManager.SetOnError(func(err error) {
			log.Printf("state persistence autosave failed: %v", err)
		})
		if err := persistManager.LoadAll(); err != nil {
			log.Fatalf("failed to load persisted state: %v", err)
		}
		persistManager.BindAutoSave()
		if err := persistManager.SaveAll(); err != nil {
			log.Fatalf("failed to save initial persisted state: %v", err)
		}
		log.Printf("state persistence enabled at %s", persistDir)
	}
	mcpStore, err := mcpregistry.NewFromEnv(nil)
	if err != nil {
		log.Fatalf("invalid mcp registry config: %v", err)
	}

	router := gateway.NewRouter(gateway.Dependencies{
		Orchestrator:    svc,
		Policy:          policy.NewDynamicEngine(settingsStore, tools),
		ModelMapper:     mapper,
		Settings:        settingsStore,
		ToolCatalog:     tools,
		SessionStore:    sessionStore,
		RunStore:        runStore,
		TodoStore:       todoStore,
		PlanStore:       planStore,
		EventStore:      eventStore,
		MCPRegistry:     mcpStore,
		SchedulerStatus: selector,
		ProbeStatus:     probeRunner,
		AdminToken:      os.Getenv("ADMIN_TOKEN"),
		RunLogger:       runLogger,
	})

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	runtimeCtx, runtimeCancel := context.WithCancel(context.Background())
	defer runtimeCancel()
	if probeRunner != nil {
		probeRunner.Start(runtimeCtx)
	}

	// Intelligence probe: runs after first probe cycle, evaluates adapter intelligence
	if upstream.ParseBoolEnv("ENABLE_TASK_DISPATCH", false) && len(adapters) > 1 {
		go func() {
			// Wait for initial probe to complete
			time.Sleep(5 * time.Second)
			log.Println("starting intelligence evaluation...")
			intelTimeout := upstream.ParseDurationEnv("INTEL_PROBE_TIMEOUT", 15*time.Second)
			scores := make([]scheduler.IntelligenceScore, 0, len(adapters))
			for _, a := range adapters {
				if a == nil {
					continue
				}
				model := ""
				if h, ok := a.(interface{ ModelHint() string }); ok {
					model = h.ModelHint()
				}
				if model == "" {
					model = "default"
				}
				result := probe.ProbeIntelligence(runtimeCtx, a, model, intelTimeout)
				log.Printf("intelligence: adapter=%s model=%s score=%.0f/100 latency=%dms",
					result.AdapterName, result.Model, result.Score, result.LatencyMS)
				for _, d := range result.Details {
					log.Printf("  %s: %.0f/20", d.Category, d.Score)
				}
				scores = append(scores, scheduler.IntelligenceScore{
					AdapterName: result.AdapterName,
					Model:       result.Model,
					Score:       result.Score,
					TestedAt:    result.TestedAt,
				})
			}
			election.UpdateScores(scores)
		}()
	}

	go func() {
		log.Printf("cc-gateway listening on :%s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	runtimeCancel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func adapterNames(adapters []upstream.Adapter) []string {
	out := make([]string, 0, len(adapters))
	for _, a := range adapters {
		if a == nil {
			continue
		}
		out = append(out, a.Name())
	}
	return out
}
