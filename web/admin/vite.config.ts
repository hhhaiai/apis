import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

export default defineConfig({
  base: "/admin/",
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      "/v1": "http://127.0.0.1:8080",
      "/admin/settings": "http://127.0.0.1:8080",
      "/admin/tools": "http://127.0.0.1:8080",
      "/admin/scheduler": "http://127.0.0.1:8080",
      "/admin/probe": "http://127.0.0.1:8080",
      "/admin/cost": "http://127.0.0.1:8080",
      "/admin/status": "http://127.0.0.1:8080",
      "/healthz": "http://127.0.0.1:8080"
    }
  }
});
