# Yaegi WebAssembly - Minimal Guide

Compile Yaegi Go interpreter to WebAssembly.

## Purpose of Wrapper

The wrapper is needed because:
- **Yaegi CLI** expects file system access and terminal I/O (won't work in browsers)
- **Custom wrapper** creates a JavaScript-accessible API using `syscall/js`
- **Bridges Go and JavaScript** by exposing functions like `window.yaegi.eval()`
- **Handles I/O properly** by capturing stdout/stderr for web environments

## Setup

```bash
mkdir yaegi-wasm && cd yaegi-wasm
go mod init yaegi-wasm
go get github.com/traefik/yaegi@latest
```

## Create Wrapper (`main.go`)

```go
package main

import (
    "bytes"
    "fmt"
    "io"
    "os"
    "syscall/js"
    "github.com/traefik/yaegi/interp"
    "github.com/traefik/yaegi/stdlib"
)

var interpreter *interp.Interpreter

func main() {
    c := make(chan struct{}, 0)
    
    global := js.Global()
    if global.Get("window").IsUndefined() {
        global.Set("window", global)
    }
    
    interpreter = interp.New(interp.Options{})
    interpreter.Use(stdlib.Symbols)
    
    global.Get("window").Set("yaegi", map[string]interface{}{
        "eval":  js.FuncOf(evalGo),
        "reset": js.FuncOf(resetInterpreter),
    })
    
    <-c
}

func evalGo(this js.Value, args []js.Value) interface{} {
    if len(args) != 1 {
        return map[string]interface{}{
            "success": false,
            "error":   "eval requires exactly one argument",
        }
    }
    
    sourceCode := args[0].String()
    
    // Capture stdout
    oldStdout := os.Stdout
    r, w, _ := os.Pipe()
    os.Stdout = w
    
    var outputBuffer bytes.Buffer
    done := make(chan bool)
    
    go func() {
        io.Copy(&outputBuffer, r)
        done <- true
    }()
    
    var evalError error
    func() {
        defer func() {
            if r := recover(); r != nil {
                evalError = fmt.Errorf("panic: %v", r)
            }
        }()
        _, evalError = interpreter.Eval(sourceCode)
    }()
    
    w.Close()
    os.Stdout = oldStdout
    <-done
    
    output := outputBuffer.String()
    
    if evalError != nil {
        return map[string]interface{}{
            "success": false,
            "error":   evalError.Error(),
            "output":  output,
        }
    }
    
    return map[string]interface{}{
        "success": true,
        "output":  output,
    }
}

func resetInterpreter(this js.Value, args []js.Value) interface{} {
    interpreter = interp.New(interp.Options{})
    interpreter.Use(stdlib.Symbols)
    return map[string]interface{}{"success": true}
}
```

## Build

```bash
export GOOS=js GOARCH=wasm
go build -o yaegi.wasm main.go
cp "$(go env GOROOT)/misc/wasm/wasm_exec.js" .
```

## Usage

### Load in HTML

```html
<script src="wasm_exec.js"></script>
<script>
async function loadYaegi() {
    const go = new Go();
    const result = await WebAssembly.instantiateStreaming(fetch("yaegi.wasm"), go.importObject);
    go.run(result.instance);
}

loadYaegi().then(() => {
    // Use window.yaegi.eval() after loading
    const result = window.yaegi.eval(`
        package main
        import "fmt"
        func main() {
            fmt.Println("Hello from Yaegi!")
        }
    `);
    
    console.log(result.success ? result.output : result.error);
});
</script>
```

### JavaScript API

```javascript
// Execute Go code
const result = window.yaegi.eval(goCode);

// Check result
if (result.success) {
    console.log("Output:", result.output);
} else {
    console.log("Error:", result.error);
}

// Reset interpreter
window.yaegi.reset();
```

## File Structure

```
yaegi-wasm/
├── main.go
├── go.mod
├── go.sum
├── yaegi.wasm      (generated)
└── wasm_exec.js    (copied from Go)
```

## Notes

- WebAssembly file will be ~10-20MB (includes Go runtime)
- Must serve over HTTP, not file:// protocol
- Takes ~100ms to initialize after loading