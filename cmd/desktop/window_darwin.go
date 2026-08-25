package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework WebKit

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

// ytsOnMain runs a block on the UI thread, without a hop when it is already
// there. AppKit refuses calls from anywhere else, and a bound JavaScript
// function is answered on the main thread already — so the common case is a
// direct call and the deferral is only insurance.
static void ytsOnMain(dispatch_block_t block) {
	if ([NSThread isMainThread]) {
		block();
	} else {
		dispatch_async(dispatch_get_main_queue(), block);
	}
}

// ytsMakeVibrant puts an NSVisualEffectView behind the web view and lets the
// content run the full height of the window.
//
// The web view starts out as the window's contentView, so it is re-parented
// rather than replaced: the effect view becomes the content, and the web view
// becomes its only subview. Everything the page then draws with an alpha below
// one is composited over the desktop by AppKit, which is the only way to get
// the real material — CSS backdrop-filter can only blur what is inside the
// page, and behind the page there is nothing.
//
// The web view must also be told to stop painting its own opaque backdrop,
// which is what `drawsBackground` does. It is not in WKWebView's public
// interface, so it is set by key and the exception is swallowed: a window that
// is merely opaque is a far better failure than one that will not open.
static void ytsMakeVibrant(void *handle) {
	ytsOnMain(^{
		NSWindow *window = (NSWindow *)handle;
		NSView *web = [window contentView];
		if (web == nil) {
			return;
		}

		// The traffic lights float over the page, and the page reserves room
		// for them: see --traffic-lights in the v2 stylesheet.
		window.titlebarAppearsTransparent = YES;
		window.titleVisibility = NSWindowTitleHidden;
		window.styleMask |= NSWindowStyleMaskFullSizeContentView;

		if ([web isKindOfClass:[WKWebView class]]) {
			@try {
				[web setValue:@NO forKey:@"drawsBackground"];
			} @catch (NSException *ignored) {
			}
			if (@available(macOS 12.0, *)) {
				((WKWebView *)web).underPageBackgroundColor = [NSColor clearColor];
			}
		}

		NSVisualEffectView *effect =
			[[NSVisualEffectView alloc] initWithFrame:[[window contentView] bounds]];
		effect.material = NSVisualEffectMaterialSidebar;
		effect.blendingMode = NSVisualEffectBlendingModeBehindWindow;
		// Following the window means the material greys out when the app is in
		// the background, exactly as every other macOS window does.
		effect.state = NSVisualEffectStateFollowsWindowActiveState;
		effect.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;

		[web retain];
		[window setContentView:effect];
		web.frame = effect.bounds;
		web.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
		[effect addSubview:web];
		[web release];
		[effect release];
	});
}

// ytsDragging guards the fallback loop below against being entered twice. The
// loop pumps the run loop, so a second script message can arrive while the
// first is still in it.
static BOOL ytsDragging = NO;

// ytsWindowDrag moves the window with the mouse until the button comes up, and
// returns whether it took the gesture.
//
// AppKit is asked to run the drag rather than being told where to put the
// window frame by frame. That is not a shortcut, it is the only way to get the
// behaviours a person expects and would not think to ask for: snapping to
// screen edges, dragging between displays with different scale factors,
// carrying the window to another Space, and the tiling gestures on Sequoia.
// A hand-rolled setFrameOrigin loop has none of them, and reads as *strange*
// long before anyone can say which one is missing.
//
// performWindowDragWithEvent: needs the mouse event that began the gesture, and
// by the time JavaScript has seen a pointerdown and posted a message across, the
// DOM event is long gone. But NSApplication still has the AppKit event it came
// from, and while the button is held that event is a mouse-down or a drag —
// exactly what is wanted. If it is neither (the message lost a race with the
// mouse-up, say) the gesture is over and there is nothing to start.
//
// The fallback is for the case where the current event exists but is not usable
// while the button is genuinely still down. It is the classic AppKit tracking
// loop: block here, pump mouse events, and move the frame by the difference in
// screen position. Less good than AppKit's own drag, and it is why the first
// branch is tried first.
static int ytsWindowDrag(void *handle) {
	if (![NSThread isMainThread]) {
		return 0;
	}
	if (ytsDragging) {
		return 1;
	}

	NSWindow *window = (NSWindow *)handle;

	// Nothing to drag if the button is already up. Checked before anything
	// else: the fallback loop would otherwise block until the *next* mouse-up,
	// freezing the window on a click that was over before it arrived.
	if (([NSEvent pressedMouseButtons] & 1) == 0) {
		return 0;
	}

	NSEvent *current = [NSApp currentEvent];
	if (current != nil) {
		NSEventType type = [current type];
		if (type == NSEventTypeLeftMouseDown || type == NSEventTypeLeftMouseDragged) {
			[window performWindowDragWithEvent:current];
			return 1;
		}
	}

	ytsDragging = YES;
	NSPoint origin = [NSEvent mouseLocation];
	NSRect frame = [window frame];
	while (([NSEvent pressedMouseButtons] & 1) != 0) {
		NSEvent *next =
			[window nextEventMatchingMask:(NSEventMaskLeftMouseDragged | NSEventMaskLeftMouseUp)
								untilDate:[NSDate dateWithTimeIntervalSinceNow:0.1]
								   inMode:NSEventTrackingRunLoopMode
								  dequeue:YES];
		if (next != nil && [next type] == NSEventTypeLeftMouseUp) {
			break;
		}
		NSPoint now = [NSEvent mouseLocation];
		[window setFrameOrigin:NSMakePoint(frame.origin.x + (now.x - origin.x),
										   frame.origin.y + (now.y - origin.y))];
	}
	ytsDragging = NO;
	return 1;
}

// ytsWindowZoom is the double-click-the-titlebar gesture.
static void ytsWindowZoom(void *handle) {
	ytsOnMain(^{
		NSWindow *window = (NSWindow *)handle;
		[window zoom:nil];
	});
}
*/
import "C"

import webview "github.com/webview/webview_go"

/*
The native half of the window.

A WKWebView is not Chromium: `-webkit-app-region: drag` does nothing, and the
only region macOS will move a window from is the AppKit titlebar — which this
window hides, because the tab strip is drawn by the page. So the page is given
the two verbs it cannot express in CSS, and hands the gesture straight back to
AppKit.
*/

// dressWindow applies the material and binds the window verbs the page calls.
//
// Called before Navigate. The web view exists from the moment the window does,
// so there is nothing to wait for — and both the user script and the bindings
// have to be installed before a page loads to reach that page at all.
func dressWindow(w webview.WebView) {
	handle := w.Window()
	C.ytsMakeVibrant(handle)

	// At document start, so the very first paint is already transparent. An
	// opaque html background would sit on top of the material until the bundle
	// evaluated, and the window would open solid and then dissolve.
	w.Init("document.documentElement.classList.add('yts-vibrancy')")

	// Errors here would mean a duplicate name, which is a bug in this file
	// rather than a condition; the page copes with a missing binding by
	// falling back to an ordinary, undraggable window.
	//
	// The drag call does not return until the mouse comes up, because that is
	// what running a drag means. Blocking the UI thread for the length of a
	// gesture is exactly what AppKit does for a native titlebar.
	_ = w.Bind("ytsWindowDrag", func() (bool, error) {
		return C.ytsWindowDrag(handle) != 0, nil
	})
	_ = w.Bind("ytsWindowZoom", func() error {
		C.ytsWindowZoom(handle)
		return nil
	})
}
