//go:build darwin && snapshot

package desktop

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

#include <stdlib.h>

// activateApp brings the app forward and, for headless verification runs
// (locked or sleeping display), stops WebKit from pausing rendering when
// macOS reports the window as occluded.
static void activateApp(void *nswindow) {
	[NSApp activateIgnoringOtherApps:YES];
	NSWindow *w = (__bridge NSWindow *)nswindow;
	WKWebView *web = [w valueForKey:@"webView"];
	@try {
		[web setValue:@NO forKey:@"_windowOcclusionDetectionEnabled"];
	} @catch (NSException *e) {
	}
}

// clickAt sends a left click at a point in the page (top-left origin) through
// AppKit's normal event dispatch (hit-testing, first responder).
static void clickAt(void *nswindow, double x, double y) {
	NSWindow *w = (__bridge NSWindow *)nswindow;
	NSPoint p = NSMakePoint(x, w.contentView.bounds.size.height - y);
	NSEventType types[] = {NSEventTypeLeftMouseDown, NSEventTypeLeftMouseUp};
	for (int i = 0; i < 2; i++) {
		NSEvent *e = [NSEvent mouseEventWithType:types[i] location:p modifierFlags:0 timestamp:NSProcessInfo.processInfo.systemUptime
		                            windowNumber:w.windowNumber context:nil eventNumber:0 clickCount:1 pressure:i == 0 ? 1 : 0];
		[NSApp sendEvent:e];
	}
}

// pressKeyApp sends a key press the way the system does (NSApp sendEvent:,
// which offers key equivalents to the key window's views first).
static void pressKeyApp(void *nswindow, int code, unsigned long mods, const char *chars) {
	NSWindow *w = (__bridge NSWindow *)nswindow;
	[NSApp activateIgnoringOtherApps:YES];
	[w makeKeyAndOrderFront:nil];
	NSString *c = [NSString stringWithUTF8String:chars];
	NSEventType types[] = {NSEventTypeKeyDown, NSEventTypeKeyUp};
	for (int i = 0; i < 2; i++) {
		NSEvent *e = [NSEvent keyEventWithType:types[i] location:NSZeroPoint modifierFlags:mods timestamp:NSProcessInfo.processInfo.systemUptime
		                          windowNumber:w.windowNumber context:nil characters:c charactersIgnoringModifiers:c.lowercaseString isARepeat:NO keyCode:code];
		[NSApp sendEvent:e];
	}
}

// pressKey sends a key press (virtual key code, modifier flags, characters).
static void pressKey(void *nswindow, int code, unsigned long mods, const char *chars) {
	NSWindow *w = (__bridge NSWindow *)nswindow;
	NSString *c = [NSString stringWithUTF8String:chars];
	NSString *plain = c.lowercaseString;
	NSEventType types[] = {NSEventTypeKeyDown, NSEventTypeKeyUp};
	for (int i = 0; i < 2; i++) {
		NSEvent *e = [NSEvent keyEventWithType:types[i] location:NSZeroPoint modifierFlags:mods timestamp:NSProcessInfo.processInfo.systemUptime
		                          windowNumber:w.windowNumber context:nil characters:c charactersIgnoringModifiers:plain isARepeat:NO keyCode:code];
		// AppKit offers key equivalents to the key window only, and a
		// window driven in the background isn't key: do what it would.
		if (i == 0 && (mods & (NSEventModifierFlagCommand | NSEventModifierFlagControl)) && !w.isKeyWindow &&
		    ([w performKeyEquivalent:e] || [NSApp.mainMenu performKeyEquivalent:e])) {
			continue;
		}
		// A real key window has an active text input context (input methods
		// such as Press and Hold see every key).
		NSTextInputContext *ctx = [w.firstResponder isKindOfClass:NSView.class] ? ((NSView *)w.firstResponder).inputContext : nil;
		if (ctx && NSTextInputContext.currentInputContext != ctx) [ctx activate];
		[w sendEvent:e];
	}
}

// captureWindow renders the app's own window into a PNG using public,
// in-process APIs: AppKit draws the window frame (traffic lights, glass
// material, titlebar), and WKWebView snapshots the page it renders in its
// separate web process. The page image is composited over the frame at the
// web view's position. Runs on the main thread; the file is written
// asynchronously when WebKit delivers the snapshot.
static void captureWindow(void *nswindow, const char *cpath) {
	NSWindow *w = (__bridge NSWindow *)nswindow;
	NSString *path = [NSString stringWithUTF8String:cpath];
	WKWebView *web = [w valueForKey:@"webView"];
	NSView *frameView = w.contentView.superview ?: w.contentView;
	NSRect bounds = frameView.bounds;

	NSBitmapImageRep *chrome = [frameView bitmapImageRepForCachingDisplayInRect:bounds];
	[frameView cacheDisplayInRect:bounds toBitmapImageRep:chrome];
	NSRect webRect = [web convertRect:web.bounds toView:frameView];

	[web takeSnapshotWithConfiguration:nil completionHandler:^(NSImage *page, NSError *err) {
		NSImage *out = [[NSImage alloc] initWithSize:bounds.size];
		[out lockFocus];
		[chrome drawInRect:bounds];
		if (page) {
			[page drawInRect:webRect fromRect:NSZeroRect operation:NSCompositingOperationSourceOver fraction:1.0];
		}
		[out unlockFocus];
		NSData *tiff = [out TIFFRepresentation];
		NSBitmapImageRep *rep = [NSBitmapImageRep imageRepWithData:tiff];
		NSData *png = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
		[png writeToFile:path atomically:YES];
	}];
}
*/
import "C"

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// MaybeRunSnapshots drives the real window from a script for visual
// verification. Only compiled with -tags snapshot; set
// YTS_SNAPSHOT_SCRIPT to a file of steps:
//
//	wait 1500          sleep (ms)
//	js <code>          run JavaScript in the page
//	shot <name>        save <dir>/<name>.png of the window
//	size <w> <h>       resize the window
//	focus              bring the app and window to the front
//	click <x> <y>      click at a point of the page (CSS px)
//	key <code> <mods> <chars>  press a key (macOS key code, NSEvent modifier flags)
//	appkey <code> <mods> <chars>  the same through NSApp, making the window key first
//	quit               quit the app
//
// Screenshots go to YTS_SNAPSHOT_DIR (default: the script's folder).
func MaybeRunSnapshots(app *application.App, win *application.WebviewWindow) {
	script := os.Getenv("YTS_SNAPSHOT_SCRIPT")
	if script == "" {
		return
	}
	dir := os.Getenv("YTS_SNAPSHOT_DIR")
	if dir == "" {
		dir = filepath.Dir(script)
	}
	go func() {
		f, err := os.Open(script)
		if err != nil {
			slog.Error("snapshot script", "err", err)
			return
		}
		defer f.Close()
		time.Sleep(2 * time.Second) // let the window appear and the UI boot
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			cmd, arg, _ := strings.Cut(line, " ")
			switch cmd {
			case "wait":
				ms, _ := strconv.Atoi(arg)
				time.Sleep(time.Duration(ms) * time.Millisecond)
			case "js":
				win.ExecJS(arg)
			case "size":
				var w, h int
				fmt.Sscan(arg, &w, &h)
				application.InvokeSync(func() { win.SetSize(w, h) })
			case "click":
				var x, y float64
				fmt.Sscan(arg, &x, &y)
				application.InvokeSync(func() { C.clickAt(win.NativeWindow(), C.double(x), C.double(y)) })
			case "appkey":
				var code int
				var mods uint64
				var chars string
				fmt.Sscan(arg, &code, &mods, &chars)
				chars = map[string]string{"RET": "\r", "SP": " ", "ESC": "\x1b", "TAB": "\t"}[chars] + map[bool]string{true: chars}[len(chars) == 1]
				cs := C.CString(chars)
				application.InvokeSync(func() { C.pressKeyApp(win.NativeWindow(), C.int(code), C.ulong(mods), cs) })
				C.free(unsafe.Pointer(cs))
			case "key":
				var code int
				var mods uint64
				var chars string
				fmt.Sscan(arg, &code, &mods, &chars)
				chars = map[string]string{"RET": "\r", "SP": " ", "ESC": "\x1b", "TAB": "\t"}[chars] + map[bool]string{true: chars}[len(chars) == 1]
				cs := C.CString(chars)
				application.InvokeSync(func() { C.pressKey(win.NativeWindow(), C.int(code), C.ulong(mods), cs) })
				C.free(unsafe.Pointer(cs))
			case "focus":
				application.InvokeSync(func() {
					C.activateApp(win.NativeWindow())
					win.Show()
					win.Focus()
				})
			case "shot":
				path := filepath.Join(dir, arg+".png")
				_ = os.Remove(path)
				cpath := C.CString(path)
				application.InvokeSync(func() { C.captureWindow(win.NativeWindow(), cpath) })
				C.free(unsafe.Pointer(cpath))
				waitForFile(path, 5*time.Second)
				slog.Info("snapshot", "path", path)
			case "quit":
				app.Quit()
				return
			}
		}
	}()
}

func waitForFile(path string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}
