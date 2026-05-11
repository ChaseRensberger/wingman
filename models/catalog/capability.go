package catalog

// CapabilityID is the canonical string vocabulary for catalog capabilities.
type CapabilityID string

const (
	CapabilityTextInput        CapabilityID = "text-input"
	CapabilityTextOutput       CapabilityID = "text-output"
	CapabilityImageInput       CapabilityID = "image-input"
	CapabilityPDFInput         CapabilityID = "pdf-input"
	CapabilityReasoning        CapabilityID = "reasoning"
	CapabilityToolCalling      CapabilityID = "tool-calling"
	CapabilityFunctionCalling  CapabilityID = "function-calling"
	CapabilityStructuredOutput CapabilityID = "structured-output"
)

// ParameterID is the canonical string vocabulary for catalog-supported params.
type ParameterID string

const (
	ParameterTemperature     ParameterID = "sampling.temperature"
	ParameterTopP            ParameterID = "sampling.top_p"
	ParameterMaxOutputTokens ParameterID = "limits.max_output_tokens"
)
