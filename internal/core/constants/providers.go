package constants

const (
	ProviderTypeOllama       = "ollama"
	ProviderTypeLemonade     = "lemonade"
	ProviderTypeLlamaCpp     = "llamacpp"
	ProviderTypeLMStudio     = "lm-studio"
	ProviderTypeOpenAI       = "openai"
	ProviderTypeOpenAICompat = "openai-compatible"
	ProviderTypeSGLang       = "sglang"
	ProviderTypeVLLM         = "vllm"
	ProviderTypeVLLMMLX      = "vllm-mlx"
	ProviderTypeDockerMR     = "docker-model-runner"

	// Provider display names
	ProviderDisplayOllama   = "Ollama"
	ProviderDisplayLemonade = "Lemonade"
	ProviderDisplayLlamaCpp = "llama.cpp"
	ProviderDisplayLMStudio = "LM Studio"
	ProviderDisplayOpenAI   = "OpenAI"
	ProviderDisplaySGLang   = "SGLang"
	ProviderDisplayVLLM     = "vLLM"
	ProviderDisplayVLLMMLX  = "vLLM-MLX"
	ProviderDisplayDockerMR = "Docker Model Runner"

	// Common provider prefixes
	// llama.cpp provider prefixes
	ProviderPrefixLlamaCpp1 = "llamacpp"
	ProviderPrefixLlamaCpp2 = "llama-cpp"
	ProviderPrefixLlamaCpp3 = "llama_cpp"
	ProviderPrefixLMStudio1 = "lmstudio"
	ProviderPrefixLMStudio2 = "lm-studio"
	ProviderPrefixLMStudio3 = "lm_studio"
	// vLLM-MLX provider prefixes
	ProviderPrefixVLLMMLX1 = "vllm-mlx"
	ProviderPrefixVLLMMLX2 = "vllmmlx"
	// Docker Model Runner provider prefix
	ProviderPrefixDockerMR = "dmr"
)
