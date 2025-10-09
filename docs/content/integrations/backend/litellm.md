---
title: LiteLLM Integration - Unified Gateway to 100+ LLM Providers
description: Configure and integrate LiteLLM with Olla for unified access to OpenAI, Anthropic, Bedrock, Azure, Google Vertex AI, and 100+ LLM providers through a single interface.
keywords: LiteLLM, Olla proxy, API gateway, OpenAI, Anthropic, Bedrock, cloud LLM, unified API, model routing
---

# LiteLLM Integration

<table>
    <tr>
        <th>Home</th>
        <td><a href="https://github.com/BerriAI/litellm">github.com/BerriAI/litellm</a></td>
    </tr>
    <tr>
        <th>Since</th>
        <td>Olla <code>v0.0.17</code></td>
    </tr>
    <tr>
        <th>Type</th>
        <td><code>litellm</code> (use in <a href="/olla/configuration/overview/#endpoint-configuration">endpoint configuration</a>)</td>
    </tr>
    <tr>
        <th>Profile</th>
        <td><code>litellm.yaml</code> (see <a href="https://github.com/thushan/olla/blob/main/config/profiles/litellm.yaml">latest</a>)</td>
    </tr>
    <tr>
        <th>Features</th>
        <td>
            <ul>
                <li>Proxy Forwarding</li>
                <li>Health Check (native)</li>
                <li>Model Unification</li>
                <li>Model Detection & Normalisation</li>
                <li>OpenAI API Compatibility</li>
                <li>100+ Provider Support</li>
                <li>Automatic API Translation</li>
                <li>Cost Tracking</li>
                <li>Response Caching</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Unsupported</th>
        <td>
            <ul>
                <li>Direct Model Management</li>
                <li>Model Download/Upload</li>
                <li>GPU Management</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Attributes</th>
        <td>
            <ul>
                <li>OpenAI Compatible</li>
                <li>Multi-Provider Gateway</li>
                <li>Automatic Fallbacks</li>
                <li>Load Balancing</li>
                <li>Spend Management</li>
            </ul>
        </td>
    </tr>
    <tr>
        <th>Prefixes</th>
        <td>
            <ul>
                <li><code>/litellm</code>(default)</li>
                <li><code>/lite</code> (disabled, enable in profile)</li>
            </ul>
            (see <a href="/olla/concepts/profile-system/#routing-prefixes">Routing Prefixes</a>)
        </td>
    </tr>
    <tr>
        <th>Endpoints</th>
        <td>
            See <a href="#endpoints-supported">below</a>
        </td>
    </tr>
</table>

## Configuration

### Basic Setup

Add LiteLLM to your Olla configuration:

```yaml
discovery:
  static:
    endpoints:
      - url: "http://localhost:4000"
        name: "litellm-gateway"
        type: "litellm"
        priority: 75
        model_url: "/v1/models"
        health_check_url: "/health"
        check_interval: 5s
        check_timeout: 2s
```

### Multiple LiteLLM Instances

Configure multiple LiteLLM servers for high availability:

```yaml
discovery:
  static:
    endpoints:
      # Primary LiteLLM instance
      - url: "http://litellm-primary:4000"
        name: "litellm-primary"
        type: "litellm"
        priority: 90
        model_url: "/v1/models"
        health_check_url: "/health"
        check_interval: 5s
        check_timeout: 2s
        
      # Secondary LiteLLM instance  
      - url: "http://litellm-secondary:4000"
        name: "litellm-secondary"
        type: "litellm"
        priority: 70
        model_url: "/v1/models"
        health_check_url: "/health"
        check_interval: 5s
        check_timeout: 2s
```

## Starting LiteLLM

LiteLLM can run in two modes:

### Basic Proxy Mode (Most Common)

Simple API translation without database - suitable for most use cases:

```bash
# Install LiteLLM
pip install 'litellm[proxy]'

# Start with environment variables
export OPENAI_API_KEY=sk-...
export ANTHROPIC_API_KEY=sk-ant-...

# Run proxy
litellm --model gpt-3.5-turbo \
        --model claude-3-haiku-20240307 \
        --port 4000
```

### Configuration File Mode

Create a `litellm_config.yaml`:

```yaml
model_list:
  # OpenAI models
  - model_name: gpt-4
    litellm_params:
      model: openai/gpt-4
      api_key: ${OPENAI_API_KEY}
  
  - model_name: gpt-3.5-turbo
    litellm_params:
      model: openai/gpt-3.5-turbo
      api_key: ${OPENAI_API_KEY}
  
  # Anthropic models
  - model_name: claude-3-opus
    litellm_params:
      model: anthropic/claude-3-opus-20240229
      api_key: ${ANTHROPIC_API_KEY}
  
  - model_name: claude-3-sonnet
    litellm_params:
      model: anthropic/claude-3-sonnet-20240229
      api_key: ${ANTHROPIC_API_KEY}
  
  # AWS Bedrock models
  - model_name: claude-3-bedrock
    litellm_params:
      model: bedrock/anthropic.claude-3-sonnet
      aws_access_key_id: ${AWS_ACCESS_KEY_ID}
      aws_secret_access_key: ${AWS_SECRET_ACCESS_KEY}
      aws_region_name: us-east-1
  
  # Google models
  - model_name: gemini-pro
    litellm_params:
      model: gemini/gemini-pro
      api_key: ${GEMINI_API_KEY}
  
  # Together AI models
  - model_name: llama-70b
    litellm_params:
      model: together_ai/meta-llama/Llama-3-70b-chat-hf
      api_key: ${TOGETHER_API_KEY}

# Optional: Caching configuration
litellm_settings:
  cache: true
  cache_ttl: 3600
  
  # Optional: Spend tracking
  max_budget: 100  # $100 budget
  budget_duration: 30d  # 30 days
```

Start with configuration:

```bash
litellm --config litellm_config.yaml --port 4000
```

## Endpoints Supported

### Core Endpoints (Always Available)

These endpoints work in all LiteLLM deployments:

| Endpoint | Method | Description | Prefix Required |
|----------|--------|-------------|-----------------|
| `/v1/chat/completions` | POST | Chat completions | `/olla/litellm` |
| `/v1/completions` | POST | Text completions | `/olla/litellm` |
| `/v1/embeddings` | POST | Generate embeddings | `/olla/litellm` |
| `/v1/models` | GET | List available models | `/olla/litellm` |
| `/health` | GET | Health check | `/olla/litellm` |

### Advanced Endpoints (Database Required)

These endpoints only work with PostgreSQL database backend:

| Endpoint | Method | Description | Requirements |
|----------|--------|-------------|--------------|
| `/key/generate` | POST | Generate API key | Database + Admin auth |
| `/user/info` | GET | User information | Database |
| `/team/info` | GET | Team information | Database |
| `/spend/calculate` | GET | Calculate spend | Database |

**Note:** Most users run LiteLLM in basic proxy mode without database, so these endpoints won't be available by default in the profile in Olla, you will have to add them in.

## Supported Providers

LiteLLM provides access to 100+ LLM providers:

### Major Cloud Providers
- **OpenAI**: GPT-5, GPT-4, GPT-3.5, Embeddings
- **Anthropic**: Claude 4.x / 3.x (Opus, Sonnet, Haiku), Claude 2
- **Google**: Gemini Pro, PaLM, Vertex AI
- **AWS Bedrock**: Claude, Llama, Mistral, Titan
- **Azure**: Azure OpenAI Service
- **Cohere**: Command, Embed

### Open Model Platforms
- **Together AI**: Llama, Mixtral, Qwen
- **Replicate**: Various open models
- **Hugging Face**: Inference API & Endpoints
- **Anyscale**: Llama, Mistral
- **Perplexity**: pplx models
- **Groq**: Fast inference

### Specialized Providers
- **Voyage AI**: Embeddings
- **AI21**: Jurassic models
- **NLP Cloud**: Various models
- **Aleph Alpha**: Luminous models
- **Databricks**: DBRX
- **DeepInfra**: Open models

## Usage Examples

### Basic Chat Completion

```bash
# Using LiteLLM through Olla
curl -X POST http://localhost:40114/olla/litellm/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

### Provider-Prefixed Models

LiteLLM supports provider-prefixed model names:

```python
import openai

client = openai.OpenAI(base_url="http://localhost:40114/olla/litellm/v1")

# Routes to OpenAI via LiteLLM
response = client.chat.completions.create(
    model="openai/gpt-4",
    messages=[{"role": "user", "content": "Hello!"}]
)

# Routes to Anthropic via LiteLLM
response = client.chat.completions.create(
    model="anthropic/claude-3-opus-20240229",
    messages=[{"role": "user", "content": "Hello!"}]
)

# Routes to AWS Bedrock via LiteLLM
response = client.chat.completions.create(
    model="bedrock/anthropic.claude-3-sonnet",
    messages=[{"role": "user", "content": "Hello!"}]
)
```

### List Available Models

```bash
# Get all models from LiteLLM
curl http://localhost:40114/olla/litellm/v1/models

# Response includes all configured models
{
  "data": [
    {"id": "gpt-4", "object": "model"},
    {"id": "claude-3-opus", "object": "model"},
    {"id": "gemini-pro", "object": "model"},
    {"id": "llama-70b", "object": "model"}
  ]
}
```

## Advanced Configuration

### Cost-Optimised Routing

Configure Olla to route to cheaper providers first:

```yaml
endpoints:
  # Local models (free)
  - url: "http://localhost:11434"
    name: "local-ollama"
    type: "ollama"
    priority: 100
  
  # LiteLLM with budget models
  - url: "http://litellm-budget:4000"
    name: "litellm-budget"
    type: "litellm"
    priority: 75
  
  # LiteLLM with premium models
  - url: "http://litellm-premium:4000"
    name: "litellm-premium"
    type: "litellm"
    priority: 50
```

### Multi-Region Setup

```yaml
endpoints:
  # US East region
  - url: "http://litellm-us-east:4000"
    name: "litellm-us-east"
    type: "litellm"
    priority: 100
  
  # EU West region
  - url: "http://litellm-eu-west:4000"
    name: "litellm-eu-west"
    type: "litellm"
    priority: 100
  
  # Load balance across regions
proxy:
  load_balancer: "least-connections"
```

## Model Capabilities

LiteLLM models are automatically categorised by Olla:

### Chat Models
- GPT-4, GPT-3.5 (OpenAI)
- Claude 3 Opus, Sonnet, Haiku (Anthropic)
- Gemini Pro (Google)
- Llama 3 (Meta via various providers)
- Mistral, Mixtral (Mistral AI)

### Embedding Models
- text-embedding-ada-002 (OpenAI)
- voyage-\* (Voyage AI)
- embed-\* (Cohere)
- titan-embed (AWS Bedrock)

### Vision Models
- GPT-4 Vision (OpenAI)
- Claude 3 models (Anthropic)
- Gemini Pro Vision (Google)

### Code Models
- GPT-4 (OpenAI)
- Claude 3 (Anthropic)
- CodeLlama (Meta via various providers)
- DeepSeek Coder (via Together AI)

## Response Headers

When requests go through LiteLLM, Olla adds tracking headers:

```
X-Olla-Endpoint: litellm-gateway
X-Olla-Backend-Type: litellm
X-Olla-Model: gpt-4
X-Olla-Request-ID: req_abc123
X-Olla-Response-Time: 2.341s
```

## Health Monitoring

Olla continuously monitors LiteLLM health:

```bash
# Check LiteLLM status
curl http://localhost:40114/internal/status/endpoints

# Response
{
  "endpoints": [
    {
      "name": "litellm-gateway",
      "url": "http://localhost:4000",
      "status": "healthy",
      "type": "litellm",
      "models_count": 25
    }
  ]
}
```

## Troubleshooting

### LiteLLM Not Starting

1. Check Python installation: `python --version`
2. Install LiteLLM: `pip install litellm`
3. Verify API keys are set in environment
4. Check port availability: `lsof -i :4000`

### Model Not Found

1. Verify model is configured in LiteLLM config
2. Check model name matches exactly
3. Ensure API key for provider is valid
4. Check provider-specific requirements

### Slow Response Times

1. Check LiteLLM logs for rate limiting
2. Monitor provider API status
3. Enable caching in LiteLLM config
4. Consider using fallback models

### Connection Errors

1. Verify LiteLLM is running: `curl http://localhost:4000/health`
2. Check firewall rules
3. Verify network connectivity
4. Check Olla can reach LiteLLM endpoint

## Best Practices

1. **Environment Variables**: Store API keys securely
   ```bash
   export OPENAI_API_KEY=sk-...
   export ANTHROPIC_API_KEY=sk-ant-...
   ```

2. **Enable Caching** (Optional): Reduce costs with in-memory response caching
   ```yaml
   litellm_settings:
     cache: true  # In-memory cache (no database required)
     cache_ttl: 3600
   ```

3. **Use Fallbacks**: Configure backup models for reliability
   ```yaml
   model_list:
     - model_name: primary-gpt4
       litellm_params:
         model: openai/gpt-4
         fallbacks: [claude-3-opus, gpt-3.5-turbo]
   ```

4. **Monitor Health**: Check LiteLLM status
   ```bash
   curl http://localhost:40114/olla/litellm/health
   ```
   
   **Note:** Budget limits and spend tracking require database backend.

## Docker Deployment

Run LiteLLM with Docker:

```yaml
# docker-compose.yml
services:
  litellm:
    image: ghcr.io/berriai/litellm:main-latest
    ports:
      - "4000:4000"
    environment:
      - OPENAI_API_KEY=${OPENAI_API_KEY}
      - ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}
    volumes:
      - ./litellm_config.yaml:/app/config.yaml
    command: --config /app/config.yaml --port 4000
```

## Integration with Other Providers

LiteLLM works seamlessly with other Olla providers:

```yaml
endpoints:
  # Local Ollama (highest priority)
  - url: "http://localhost:11434"
    type: "ollama"
    priority: 100
  
  # LM Studio (medium priority)
  - url: "http://localhost:1234"
    type: "lm-studio"
    priority: 75
  
  # LiteLLM for cloud (lower priority)
  - url: "http://localhost:4000"
    type: "litellm"
    priority: 50
```

This setup provides:
1. Local model preference for speed and cost
2. Automatic fallback to cloud APIs
3. Unified API across all providers
4. Single endpoint for all models

## See Also

- [LiteLLM Documentation](https://docs.litellm.ai/)
- [Provider Setup Guides](https://docs.litellm.ai/docs/providers)
- [Model Pricing](https://docs.litellm.ai/docs/providers/pricing)
- [Olla Profile System](../../concepts/profile-system.md)
- [Load Balancing Guide](../../concepts/load-balancing.md)