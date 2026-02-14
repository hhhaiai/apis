$ curl 'http://localhost:5025/v1/chat/completions' \
>   -H 'Accept: */*' \
>   -H 'Accept-Language: zh-CN' \
>   -H 'Connection: keep-alive' \
>   -H 'Sec-Fetch-Dest: empty' \
>   -H 'Sec-Fetch-Mode: cors' \
>   -H 'Sec-Fetch-Site: cross-site' \
>   -H 'User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) CherryStudio/1.7.19 Chrome/140.0.7339.249 Electron/38.7.0 Safari/537.36' \
>   -H 'authorization: Bearer free' \
>   -H 'content-type: application/json' \
>   -H 'http-referer: https://cherry-ai.com' \
>   -H 'sec-ch-ua: "Not=A?Brand";v="24", "Chromium";v="140"' \
>   -H 'sec-ch-ua-mobile: ?0' \
>   -H 'sec-ch-ua-platform: "macOS"' \
>   -H 'x-title: Cherry Studio' \
>   --data-raw '{"model":"GLM-5","messages":[{"role":"system","content":"test"},{"role":"user","content":"hi"}],"stream":true,"stream_options":{"include_usage":true}}'

data: {"id":"chatcmpl-4abd424b-4c98-4ec8-8d61-f5f1c","object":"chat.completion.chunk","created":1771086001,"model":"glm-5","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}

data: {"id":"chatcmpl-4abd424b-4c98-4ec8-8d61-f5f1c","object":"chat.completion.chunk","created":1771086002,"model":"glm-5","choices":[{"index":0,"delta":{"content":"! How can I help you"},"finish_reason":null}]}

data: {"id":"chatcmpl-4abd424b-4c98-4ec8-8d61-f5f1c","object":"chat.completion.chunk","created":1771086002,"model":"glm-5","choices":[{"index":0,"delta":{"content":" today?"},"finish_reason":null}]}

data: {"id":"chatcmpl-4abd424b-4c98-4ec8-8d61-f5f1c","object":"chat.completion.chunk","created":1771086002,"model":"glm-5","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]



$ curl 'http://localhost:5022/v1/chat/completions' \
>   -H 'Accept: */*' \
>   -H 'Accept-Language: zh-CN' \
>   -H 'Connection: keep-alive' \
>   -H 'Sec-Fetch-Dest: empty' \
>   -H 'Sec-Fetch-Mode: cors' \
>   -H 'Sec-Fetch-Site: cross-site' \
>   -H 'User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) CherryStudio/1.7.19 Chrome/140.0.7339.249 Electron/38.7.0 Safari/537.36' \
>   -H 'authorization: Bearer free' \
>   -H 'content-type: application/json' \
>   -H 'http-referer: https://cherry-ai.com' \
>   -H 'sec-ch-ua: "Not=A?Brand";v="24", "Chromium";v="140"' \
>   -H 'sec-ch-ua-mobile: ?0' \
>   -H 'sec-ch-ua-platform: "macOS"' \
>   -H 'x-title: Cherry Studio' \
>   --data-raw '{"model":"GLM-4.7","messages":[{"role":"system","content":"test"},{"role":"user","content":"hi"}],"stream":true,"stream_options":{"include_usage":true}}'
data: {"id":"chatcmpl-1e75bbd3-4d2a-46e9-aa48-1089f","object":"chat.completion.chunk","created":1771086044,"model":"glm-4.7","choices":[{"index":0,"delta":{"content":"Hi there! How can I"},"finish_reason":null}]}

data: {"id":"chatcmpl-1e75bbd3-4d2a-46e9-aa48-1089f","object":"chat.completion.chunk","created":1771086044,"model":"glm-4.7","choices":[{"index":0,"delta":{"content":" help you today?"},"finish_reason":null}]}

data: {"id":"chatcmpl-1e75bbd3-4d2a-46e9-aa48-1089f","object":"chat.completion.chunk","created":1771086044,"model":"glm-4.7","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]