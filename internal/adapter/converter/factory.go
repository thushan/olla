package converter

import (
	"fmt"

	"github.com/thushan/olla/internal/core/ports"
)

// ConverterFactory creates model response converters
type ConverterFactory struct {
	converters map[string]ports.ModelResponseConverter
}

// NewConverterFactory creates a new converter factory
func NewConverterFactory() *ConverterFactory {
	factory := &ConverterFactory{
		converters: make(map[string]ports.ModelResponseConverter),
	}

	// Register all converters
	factory.RegisterConverter(NewUnifiedConverter())
	factory.RegisterConverter(NewOpenAIConverter())
	factory.RegisterConverter(NewOllamaConverter())
	factory.RegisterConverter(NewLemonadeConverter())
	factory.RegisterConverter(NewLMStudioConverter())
	factory.RegisterConverter(NewSGLangConverter())
	factory.RegisterConverter(NewVLLMConverter())

	return factory
}

// RegisterConverter registers a new converter
func (f *ConverterFactory) RegisterConverter(converter ports.ModelResponseConverter) {
	f.converters[converter.GetFormatName()] = converter
}

// GetConverter returns a converter for the specified format
func (f *ConverterFactory) GetConverter(format string) (ports.ModelResponseConverter, error) {
	// Default to unified format if not specified
	if format == "" {
		format = "unified"
	}

	converter, exists := f.converters[format]
	if !exists {
		return nil, &ports.QueryParameterError{
			Parameter: "format",
			Value:     format,
			Reason:    fmt.Sprintf("unsupported format. Supported formats: %s", f.getSupportedFormats()),
		}
	}

	return converter, nil
}

// GetSupportedFormats returns a list of supported format names
func (f *ConverterFactory) GetSupportedFormats() []string {
	formats := make([]string, 0, len(f.converters))
	for name := range f.converters {
		formats = append(formats, name)
	}
	return formats
}

// getSupportedFormats returns a comma-separated string of supported formats
func (f *ConverterFactory) getSupportedFormats() string {
	formats := f.GetSupportedFormats()
	result := ""
	for i, format := range formats {
		if i > 0 {
			result += ", "
		}
		result += format
	}
	return result
}
