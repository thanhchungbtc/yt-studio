//go:build darwin

#import <Cocoa/Cocoa.h>

extern void goAccessibilityChanged(void);

// accessibilityFlags reports macOS display accessibility settings as bits:
// 1 reduce transparency, 2 reduce motion, 4 increase contrast.
int accessibilityFlags(void) {
	NSWorkspace *ws = [NSWorkspace sharedWorkspace];
	int flags = 0;
	if (ws.accessibilityDisplayShouldReduceTransparency) flags |= 1;
	if (ws.accessibilityDisplayShouldReduceMotion) flags |= 2;
	if (ws.accessibilityDisplayShouldIncreaseContrast) flags |= 4;
	return flags;
}

// observeAccessibility calls goAccessibilityChanged whenever those settings change.
void observeAccessibility(void) {
	[[[NSWorkspace sharedWorkspace] notificationCenter]
		addObserverForName:NSWorkspaceAccessibilityDisplayOptionsDidChangeNotification
		            object:nil
		             queue:nil
		        usingBlock:^(NSNotification *note) {
			goAccessibilityChanged();
		}];
}
