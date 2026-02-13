package constants

// TranslatorMode represents how the request was handled by the translator
type TranslatorMode string

const (
	// TranslatorModePassthrough indicates request was passed through natively (no translation)
	TranslatorModePassthrough TranslatorMode = "passthrough"

	// TranslatorModeTranslation indicates request was translated between formats
	TranslatorModeTranslation TranslatorMode = "translation"
)

// TranslatorFallbackReason explains why passthrough wasn't used
type TranslatorFallbackReason string

const (
	// FallbackReasonNone indicates no fallback occurred (passthrough succeeded)
	FallbackReasonNone TranslatorFallbackReason = ""

	// FallbackReasonNoCompatibleEndpoints means no endpoints support native format
	FallbackReasonNoCompatibleEndpoints TranslatorFallbackReason = "no_compatible_endpoints"

	// FallbackReasonTranslatorDoesNotSupportPassthrough means translator lacks passthrough capability
	FallbackReasonTranslatorDoesNotSupportPassthrough TranslatorFallbackReason = "translator_does_not_support_passthrough"

	// FallbackReasonCannotPassthrough means endpoints don't support native format
	//nolint:gosec // false positive: "passthrough" is not a credential
	FallbackReasonCannotPassthrough TranslatorFallbackReason = "cannot_passthrough"
)
