# Testing Guide

## Overview

The muster codebase provides two complementary mock testing approaches for AI interactions:

1. **MockInvokeAI** — Fast unit-level mocking that replaces the `ai.InvokeAI` function entirely
2. **NewMockAITool** — Integration-level mocking with a real compiled binary for end-to-end testing

Both approaches are thread-safe, support table-driven tests, and integrate with the `testutil` package's fixtures.

## When to Use Each Approach

### Use MockInvokeAI for:
- **Unit tests** — testing individual functions that call `ai.InvokeAI`
- **Fast feedback loops** — no binary compilation overhead
- **Testing error handling** — easy to simulate different failure modes
- **Validating JSON parsing** — control exact AI responses
- **Multi-step workflows** — queue different responses for sequential calls

### Use NewMockAITool for:
- **Integration tests** — testing the full AI invocation pipeline
- **Tool binary behavior** — validating flags, environment variables, skill file reading
- **Subprocess handling** — testing timeouts, exit codes, stderr capture
- **End-to-end scenarios** — realistic execution with actual tool processes

## MockInvokeAI: Unit-Level Mocking

### Basic Usage

```go
func TestMyFunction(t *testing.T) {
    cleanup := testutil.MockInvokeAI(`{"result": "success"}`, nil)
    defer cleanup()

    // Your test code here - ai.InvokeAI now returns the mocked response
    result, err := myFunctionThatCallsAI()
    require.NoError(t, err)
    assert.Equal(t, "success", result)
}
```

### Queue Multiple Responses

For functions that make multiple AI calls:

```go
func TestMultiStepWorkflow(t *testing.T) {
    cleanup := testutil.MockInvokeAIWithQueue(
        testutil.MockResponse{Response: `{"step": "plan"}`, Err: nil},
        testutil.MockResponse{Response: `{"step": "execute"}`, Err: nil},
        testutil.MockResponse{Response: `{"step": "verify"}`, Err: nil},
    )
    defer cleanup()

    // Each call to ai.InvokeAI consumes the next response
    workflow, err := runWorkflow()
    require.NoError(t, err)
    assert.Len(t, workflow.Steps, 3)
}
```

### Tracking Invocation Count

```go
func TestAICallFrequency(t *testing.T) {
    testutil.ResetInvokeCount()
    cleanup := testutil.MockInvokeAI(`{"result": "ok"}`, nil)
    defer cleanup()

    processItems(items)

    // Verify we didn't make redundant AI calls
    assert.Equal(t, int32(5), testutil.GetInvokeCount())
}
```

## NewMockAITool: Integration-Level Mocking

### Basic Usage

```go
func TestWithMockBinary(t *testing.T) {
    mock := testutil.NewMockAITool(t, `{"processed": true}`)

    cfg := mock.InvokeConfig("test prompt")
    result, err := ai.InvokeAI(cfg)

    require.NoError(t, err)
    assert.Equal(t, `{"processed": true}`, result.RawOutput)
}
```

### Simulating Errors

```go
func TestToolFailure(t *testing.T) {
    mock := testutil.NewMockAITool(t, "")
    errorMock := mock.WithError(1, "tool execution failed")

    cfg := errorMock.InvokeConfig("test prompt")
    result, err := ai.InvokeAI(cfg)

    require.Error(t, err)
    assert.Contains(t, err.Error(), "exit status 1")
}
```

### Testing Timeouts

```go
func TestTimeout(t *testing.T) {
    mock := testutil.NewMockAITool(t, `{"slow": true}`)
    delayMock := mock.WithDelay(5 * time.Second)

    cfg := delayMock.InvokeConfig("test prompt")
    cfg.Timeout = 100 * time.Millisecond

    result, err := ai.InvokeAI(cfg)
    require.Error(t, err)
    assert.Contains(t, err.Error(), "timeout")
}
```

## Using Fixtures

The `testutil` package provides pre-defined JSON fixtures for common scenarios:

```go
func TestRoadmapValidation(t *testing.T) {
    cleanup := testutil.MockInvokeAI(testutil.ValidRoadmapItemJSON, nil)
    defer cleanup()

    item, err := parseRoadmapItem()
    require.NoError(t, err)
    assert.Equal(t, "test-feature", item.Slug)
}

func TestInvalidJSON(t *testing.T) {
    cleanup := testutil.MockInvokeAI(testutil.InvalidJSON, nil)
    defer cleanup()

    _, err := parseRoadmapItem()
    require.Error(t, err)
    assert.Contains(t, err.Error(), "parse")
}
```

## Table-Driven Tests with Mocks

```go
func TestFuzzyMatching(t *testing.T) {
    tests := []struct {
        name          string
        mockResponse  string
        expectMatch   bool
        expectPrompt  bool
    }{
        {
            name:         "high confidence auto-accept",
            mockResponse: testutil.HighConfidenceMatch,
            expectMatch:  true,
            expectPrompt: false,
        },
        {
            name:         "low confidence requires confirmation",
            mockResponse: testutil.LowConfidenceMatch,
            expectMatch:  true,
            expectPrompt: true,
        },
        {
            name:         "empty response no match",
            mockResponse: testutil.EmptyResponse,
            expectMatch:  false,
            expectPrompt: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            cleanup := testutil.MockInvokeAI(tt.mockResponse, nil)
            defer cleanup()

            match, prompted := findMatch("new-feature")
            assert.Equal(t, tt.expectMatch, match != nil)
            assert.Equal(t, tt.expectPrompt, prompted)
        })
    }
}
```

## Thread Safety

**⚠️ WARNING: Not all mocking approaches are thread-safe!**

### MockInvokeAI: NOT Parallel-Safe

`MockInvokeAI` and `MockInvokeAIWithQueue` modify the global `ai.InvokeAI` function and are **NOT safe for parallel test execution**. While the invocation counter uses atomic operations, the function replacement itself causes data races when multiple goroutines install mocks concurrently.

```go
// Safe: Sequential subtests
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        cleanup := testutil.MockInvokeAI(tt.response, nil)
        defer cleanup()
        // test code
    })
}

// Unsafe: Parallel subtests with global mock
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        t.Parallel() // DON'T DO THIS with MockInvokeAI - causes data races!
        cleanup := testutil.MockInvokeAI(tt.response, nil)
        defer cleanup()
        // test code
    })
}
```

### NewMockAITool: Partially Safe

**Binary compilation is thread-safe** (`sync.Once`), but **environment variable configuration in `InvokeConfig()` is not**. The method uses `os.Setenv()` which modifies process-global state.

**Do not use `NewMockAITool` with `t.Parallel()` subtests** unless you carefully manage environment variables with `t.Setenv()` in each test instead of relying on `InvokeConfig()`.

```go
// Unsafe: Parallel tests sharing InvokeConfig()
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        t.Parallel() // DON'T DO THIS - InvokeConfig() uses os.Setenv()
        mock := testutil.NewMockAITool(t, tt.response)
        cfg := mock.InvokeConfig("test")
        // test code
    })
}
```

For truly parallel-safe tests, use `t.Setenv()` to manage mock environment variables instead of relying on `InvokeConfig()`.
