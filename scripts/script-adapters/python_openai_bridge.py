#!/usr/bin/env python3
import json
import os
import sys
import urllib.error
import urllib.request


def fail(message):
    sys.stderr.write(message + "\n")
    sys.exit(1)


def to_text(value):
    if value is None:
        return ""
    if isinstance(value, str):
        return value
    return json.dumps(value, ensure_ascii=False)


def blocks_to_openai_messages(role, content):
    if isinstance(content, str):
        return [{"role": role, "content": content}]
    if not isinstance(content, list):
        return [{"role": role, "content": to_text(content)}]

    if role == "assistant":
        text_parts = []
        tool_calls = []
        for block in content:
            if not isinstance(block, dict):
                text_parts.append(to_text(block))
                continue
            btype = block.get("type")
            if btype == "text":
                text_parts.append(to_text(block.get("text")))
                continue
            if btype == "tool_use":
                tool_calls.append(
                    {
                        "id": block.get("id") or "",
                        "type": "function",
                        "function": {
                            "name": block.get("name") or "",
                            "arguments": json.dumps(block.get("input") or {}, ensure_ascii=False),
                        },
                    }
                )
        msg = {"role": "assistant"}
        msg["content"] = "\n".join([x for x in text_parts if x]) if text_parts else ""
        if tool_calls:
            msg["tool_calls"] = tool_calls
        return [msg]

    if role == "user":
        out = []
        text_parts = []
        for block in content:
            if not isinstance(block, dict):
                text_parts.append(to_text(block))
                continue
            btype = block.get("type")
            if btype == "tool_result":
                call_id = block.get("tool_use_id") or block.get("id") or ""
                out.append(
                    {
                        "role": "tool",
                        "tool_call_id": call_id,
                        "content": to_text(block.get("content")),
                    }
                )
                continue
            if btype == "text":
                text_parts.append(to_text(block.get("text")))
            else:
                text_parts.append(to_text(block))
        if text_parts:
            out.insert(0, {"role": "user", "content": "\n".join(text_parts)})
        return out or [{"role": "user", "content": ""}]

    return [{"role": role, "content": to_text(content)}]


def canonical_to_openai_messages(system_prompt, messages):
    out = []
    if system_prompt is not None and to_text(system_prompt):
        out.append({"role": "system", "content": to_text(system_prompt)})
    for message in messages or []:
        role = (message.get("role") or "user").strip() or "user"
        out.extend(blocks_to_openai_messages(role, message.get("content")))
    return out


def canonical_to_openai_tools(tools):
    out = []
    for tool in tools or []:
        schema = tool.get("input_schema") or {"type": "object", "properties": {}}
        desc = (tool.get("description") or "").strip() or "No description provided"
        out.append(
            {
                "type": "function",
                "function": {
                    "name": tool.get("name") or "",
                    "description": desc,
                    "parameters": schema,
                },
            }
        )
    return out


def parse_openai_response(obj):
    choices = obj.get("choices") or []
    choice = choices[0] if choices else {}
    message = choice.get("message") or {}

    blocks = []
    content = message.get("content")
    if isinstance(content, str) and content:
        blocks.append({"type": "text", "text": content})

    for tc in message.get("tool_calls") or []:
        function = tc.get("function") or {}
        args = function.get("arguments")
        parsed_args = {}
        if isinstance(args, str) and args.strip():
            try:
                parsed_args = json.loads(args)
            except json.JSONDecodeError:
                parsed_args = {"_raw": args}
        elif isinstance(args, dict):
            parsed_args = args
        blocks.append(
            {
                "type": "tool_use",
                "id": tc.get("id") or "",
                "name": function.get("name") or "",
                "input": parsed_args,
            }
        )

    finish_reason = (choice.get("finish_reason") or "").strip().lower()
    stop_reason = "end_turn"
    if finish_reason in ("tool_calls", "function_call"):
        stop_reason = "tool_use"
    elif finish_reason == "length":
        stop_reason = "max_tokens"

    usage = obj.get("usage") or {}
    return {
        "model": obj.get("model") or "",
        "blocks": blocks,
        "stop_reason": stop_reason,
        "usage": {
            "input_tokens": int(usage.get("prompt_tokens") or 0),
            "output_tokens": int(usage.get("completion_tokens") or 0),
        },
    }


def main():
    try:
        envelope = json.loads(sys.stdin.read() or "{}")
    except json.JSONDecodeError as e:
        fail(f"invalid envelope json: {e}")
    mode = (envelope.get("mode") or "complete").strip().lower()
    request = envelope.get("request") or {}

    model = os.getenv("OPENAI_MODEL", "").strip() or request.get("model") or "gpt-4o-mini"
    openai_payload = {
        "model": model,
        "max_tokens": int(request.get("max_tokens") or 1024),
        "messages": canonical_to_openai_messages(request.get("system"), request.get("messages")),
    }
    tools = canonical_to_openai_tools(request.get("tools"))
    if tools:
        openai_payload["tools"] = tools

    base_url = os.getenv("OPENAI_BASE_URL", "http://127.0.0.1:8000").rstrip("/")
    endpoint = os.getenv("OPENAI_ENDPOINT", "/v1/chat/completions")
    timeout = int(os.getenv("OPENAI_TIMEOUT_SEC", "120"))
    url = base_url + endpoint

    headers = {"Content-Type": "application/json"}
    api_key = os.getenv("OPENAI_API_KEY", "").strip()
    if api_key:
        headers["Authorization"] = f"Bearer {api_key}"

    req = urllib.request.Request(
        url=url,
        data=json.dumps(openai_payload, ensure_ascii=False).encode("utf-8"),
        headers=headers,
        method="POST",
    )

    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode("utf-8")
    except urllib.error.HTTPError as e:
        body = e.read().decode("utf-8", errors="replace")
        fail(f"http error {e.code}: {body}")
    except Exception as e:
        fail(f"request failed: {e}")

    try:
        upstream = json.loads(raw)
    except json.JSONDecodeError as e:
        fail(f"invalid upstream json: {e}; raw={raw[:500]}")
    parsed = parse_openai_response(upstream)
    if mode == "stream":
        print(json.dumps({"response": parsed}, ensure_ascii=False))
    else:
        print(json.dumps(parsed, ensure_ascii=False))


if __name__ == "__main__":
    main()
