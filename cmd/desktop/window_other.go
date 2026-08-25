//go:build !darwin

package main

import webview "github.com/webview/webview_go"

// dressWindow does nothing off macOS.
//
// Only macOS is supported, and the vibrancy and the window verbs are AppKit to
// the bone. The stub keeps the package building elsewhere — `go vet ./...` on
// a Linux CI box is worth ten lines — and the page notices the missing
// bindings and behaves as an ordinary window.
func dressWindow(webview.WebView) {}
