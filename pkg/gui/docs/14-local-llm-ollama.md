---
id: 14-local-llm-ollama
title: Setting Up Ollama for Local LLMs
category: Local AI & Self-Hosting
order: 14
description: Hardware requirements, native/Docker setup, recommended models for tabletop RPGs, and connecting Ollama to LocalRPG.
---

# Setting Up Ollama for Local LLMs

Ollama is a lightweight, cross-platform runner for open-weights large language models. It provides out-of-the-box hardware acceleration (CUDA, ROCm, Apple Metal) and exposes an OpenAI-compatible HTTP API on port `11434`. It is the easiest way to power LocalRPG's GM, Narrator, and Extractor agents without sending data to cloud APIs.

## 1. Hardware Requirements

LLM performance depends directly on parameter count and quantization:

| Model Tier | Representative Models | VRAM / RAM | Target Hardware |
| --- | --- | --- | --- |
| **Small & Fast** | `llama3.2:3b`, `qwen2.5:3b` | 4–6 GB | Modern CPUs, entry-level laptops, Apple Silicon (M1/M2 8 GB+) |
| **Recommended Baseline** | `llama3.1:8b`, `qwen2.5:7b` | 8–12 GB | NVIDIA RTX 3060/4060, Apple Silicon (16 GB+), mid-tier GPUs |
| **High Fidelity** | `qwen2.5:14b`, `mistral-small` | 16–24 GB | NVIDIA RTX 3090/4090, Apple Silicon (24 GB+) |

> [!TIP]
> For turn-based narrative roleplaying, 8-billion-parameter models like `llama3.1:8b` or `qwen2.5:7b` provide the optimal balance between creative roleplay, markdown formatting compliance, and low turn latency.

## 2. Running Ollama

### Option A: Native Installation (Recommended for Desktop)

Install Ollama directly on your operating system for optimal GPU detection:

- **macOS / Windows**: Download the installer from [ollama.com](https://ollama.com).
- **Linux**: Run the official installation script:

```bash
curl -fsSL https://ollama.com/install.sh | sh
```

Once installed, pull your chosen model from your terminal:

```bash
ollama pull llama3.2
```

Ollama automatically starts as a background service on `http://localhost:11434`.

### Option B: Running via Docker

If you prefer containerized isolation or are running on a headless home server:

```bash
docker run -d \
  --name ollama \
  --restart always \
  --gpus all \
  -v ollama:/root/.ollama \
  -p 11434:11434 \
  ollama/ollama:latest
```

Pull the model inside the container:

```bash
docker exec -it ollama ollama run llama3.2
```

Or run via `docker-compose.yml`:

```yaml
services:
  ollama:
    image: ollama/ollama:latest
    container_name: ollama
    restart: always
    ports:
      - "11434:11434"
    volumes:
      - ollama:/root/.ollama
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu]

volumes:
  ollama:
```

## 3. Verify Server Health

Test that Ollama's OpenAI-compatible endpoint responds:

```bash
curl -s http://localhost:11434/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "llama3.2",
    "messages": [
      {"role": "user", "content": "Respond with: Ready for adventure."}
    ],
    "temperature": 0.7
  }'
```

Verify that the response JSON contains `Ready for adventure.`.

## 4. Connecting in LocalRPG

### Via Settings Studio (GUI)

1. Open **Settings Studio** -> **Agents**.
2. Under **Agent Roles** (such as **GM** or **Narrator**), open the preset dropdown and choose **Ollama**.
3. The endpoint defaults to `http://localhost:11434/v1` and the model to `llama3.2`.
4. If you pulled a different model (such as `llama3.1:8b` or `qwen2.5:7b`), type its tag into the **Model** field.
5. Click **Save Settings**.

### Via Configuration File (`config.yaml` or `localrpg.yaml`)

```yaml
agents:
  roles:
    gm:
      type: http
      endpoint: "http://localhost:11434/v1"
      model: "llama3.2"
      temperature: 0.7
      max_tokens: 1024
    narrator:
      type: http
      endpoint: "http://localhost:11434/v1"
      model: "llama3.2"
      temperature: 0.7
      max_tokens: 1024
```

## 5. Alternatives: LM Studio & vLLM

LocalRPG's OpenAI HTTP client works with any standard OpenAI-compatible local server:

- **LM Studio**: Run the LM Studio desktop application, load any GGUF model, and click the **Local Server** icon to start serving on `http://localhost:1234/v1`. Select preset `lm-studio` in LocalRPG.
- **vLLM**: For multi-turn throughput and high-concurrency batching on Linux with NVIDIA GPUs, run `vllm serve meta-llama/Llama-3.1-8B-Instruct --port 8000`. Use endpoint `http://localhost:8000/v1` in LocalRPG.
