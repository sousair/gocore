package telemetry

// genai.go — single home for the experimental gen_ai.* names (rename churn = one-file fix).
// Source: github.com/open-telemetry/semantic-conventions-genai — the gen_ai
// conventions moved out of the main semantic-conventions repo, which now serves a
// redirect stub.
const (
	AttrGenAIOperationName = "gen_ai.operation.name" // "chat" | "embeddings" | "transcription" | "execute_tool"
	AttrGenAIProviderName  = "gen_ai.provider.name"  // "openrouter" | "whispercpp" | "tei"
	AttrGenAIRequestModel  = "gen_ai.request.model"
	AttrGenAIResponseModel = "gen_ai.response.model"
	AttrGenAIInputTokens   = "gen_ai.usage.input_tokens"
	AttrGenAIOutputTokens  = "gen_ai.usage.output_tokens"
	AttrGenAICostUSD       = "gen_ai.usage.cost_usd" // reserved: client lib doesn't expose OpenRouter cost yet

	AttrGenAIToolName        = "gen_ai.tool.name"
	AttrGenAIToolCallID      = "gen_ai.tool.call.id"
	AttrGenAIToolDescription = "gen_ai.tool.description"
	AttrGenAIToolType        = "gen_ai.tool.type" // "function" | "extension" | "datastore"
	AttrErrorType            = "error.type"

	// gen_ai.tool.call.arguments and gen_ai.tool.call.result are deliberately
	// absent. The spec marks them Opt-In because they carry the call payload —
	// for a finance agent that is transaction amounts, merchant names and
	// account ids, which do not belong in a span.

	// AttrGenAIRound is a local extension, not semconv: the agentic loop's round
	// index, so loop depth and max-round exhaustion are queryable.
	AttrGenAIRound = "gen_ai.round"

	// AttrGenAIFlowName names the application-level flow that made the call.
	// Not in the semantic conventions: two flows routinely share one model, so
	// the model alone cannot attribute cost or behavior to a call site.
	AttrGenAIFlowName = "gen_ai.flow.name"

	// AttrGenAIFinishReason is the provider's stop reason, verbatim. A "length"
	// finish with no visible content is a paid-for-nothing call and is
	// otherwise indistinguishable from a normal return.
	AttrGenAIFinishReason = "gen_ai.response.finish_reason"

	// AttrGenAIReasoningTokens is the share of output tokens spent on reasoning
	// the caller never receives as text.
	AttrGenAIReasoningTokens = "gen_ai.usage.reasoning_tokens"

	// OpExecuteTool is the gen_ai.operation.name value for a tool execution.
	OpExecuteTool = "execute_tool"
)
