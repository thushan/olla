# For integration_test.go - keep first (line 29), remove lines 49-55, 96-102, 388-394, 851-857, 1066-1072, 1115-1121, 1199-1205
/^func createTestConfig() config.AnthropicTranslatorConfig {$/{
    N
    N
    N
    N
    N
    N
    /func createTestConfig.*\n.*return config.AnthropicTranslatorConfig{\n.*Enabled:.*true,\n.*MaxMessageSize:.*10.*20,.*10MB\n.*StreamAsync:.*false,\n.*}\n.*}$/{
        h
        n
        x
        s/.*//
        x
    }
}
