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
	// Prevent the program from exiting
	c := make(chan struct{}, 0)

	// Fake `window` if missing (Node.js or non-browser runtime)
	global := js.Global()
	if global.Get("window").IsUndefined() {
		global.Set("window", global)
	}

	// Initialize Yaegi interpreter
	interpreter = interp.New(interp.Options{})
	interpreter.Use(stdlib.Symbols)

	// Expose JavaScript functions under `window.yaegi`
	global.Get("window").Set("yaegi", map[string]interface{}{
		"eval":    js.FuncOf(evalGo),
		"version": js.FuncOf(getVersion),
		"reset":   js.FuncOf(resetInterpreter),
	})

	// Signal that Yaegi is ready
	global.Get("console").Call("log", "Yaegi WebAssembly initialized!")

	<-c // Keep the program running
}

func evalGo(this js.Value, args []js.Value) interface{} {
	if len(args) != 1 {
		return map[string]interface{}{
			"success": false,
			"error":   "eval requires exactly one argument (Go source code)",
		}
	}

	sourceCode := args[0].String()

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Buffer to capture output
	var outputBuffer bytes.Buffer
	done := make(chan bool)

	// Read from pipe in goroutine
	go func() {
		io.Copy(&outputBuffer, r)
		done <- true
	}()

	var evalError error

	// Execute the Go code
	func() {
		defer func() {
			if r := recover(); r != nil {
				evalError = fmt.Errorf("panic: %v", r)
			}
		}()
		_, evalError = interpreter.Eval(sourceCode)
	}()

	// Restore stdout
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
		"error":   nil,
	}
}

func getVersion(this js.Value, args []js.Value) interface{} {
	return "Yaegi WebAssembly v1.0"
}

func resetInterpreter(this js.Value, args []js.Value) interface{} {
	interpreter = interp.New(interp.Options{})
	interpreter.Use(stdlib.Symbols)

	return map[string]interface{}{
		"success": true,
		"message": "Interpreter reset successfully",
	}
}
