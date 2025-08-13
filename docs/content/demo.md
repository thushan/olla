---
title: Demo - Olla Demo
description: Demo of Olla in action with VHS Tapes
keywords: olla demo, preview, demo, olla running
---

# Demo

The following is a demo of Olla recorded with [VHS](https://vhs.charm.sh/).

<div align="center">
  <img src="assets/demos/olla-v1.0.x-demo.gif" alt="Olla - LLM Proxy & Load Balancer" style="max-width: 100%; height: auto;">
</div>

The demonstration shows:

- Loading a custom configuration at startup
  - The configuration has several instances of Ollama and LMStudio
  - Only 1 is available at startup - `mac-ollama`
- Ollama request for Tinyllama is received & streamed.
- Endpoint `beehive-ollama` comes online
- New Ollama request is sent for Tinyllama is received and streamed.