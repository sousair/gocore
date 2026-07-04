package telemetry

// genai.go — single home for the experimental gen_ai.* names (rename churn = one-file fix)
const (
	AttrGenAIOperationName = "gen_ai.operation.name" // "chat" | "embeddings" | "transcription"
	AttrGenAIProviderName  = "gen_ai.provider.name"  // "openrouter" | "whispercpp" | "tei"
	AttrGenAIRequestModel  = "gen_ai.request.model"
	AttrGenAIResponseModel = "gen_ai.response.model"
	AttrGenAIInputTokens   = "gen_ai.usage.input_tokens"
	AttrGenAIOutputTokens  = "gen_ai.usage.output_tokens"
	AttrGenAICostUSD       = "gen_ai.usage.cost_usd" // reserved: client lib doesn't expose OpenRouter cost yet
)
