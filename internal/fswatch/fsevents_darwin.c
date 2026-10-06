//go:build darwin

#include <CoreServices/CoreServices.h>
#include <dispatch/dispatch.h>
#include <stdint.h>

extern void goFSEvents(uintptr_t handle, char **paths, int count, int rescan);

static void onEvents(ConstFSEventStreamRef stream, void *info, size_t count, void *eventPaths,
                     const FSEventStreamEventFlags flags[], const FSEventStreamEventId ids[]) {
	int rescan = 0;
	for (size_t i = 0; i < count; i++) {
		if (flags[i] & (kFSEventStreamEventFlagMustScanSubDirs | kFSEventStreamEventFlagRootChanged |
		                kFSEventStreamEventFlagUserDropped | kFSEventStreamEventFlagKernelDropped)) {
			rescan = 1;
		}
	}
	goFSEvents((uintptr_t)info, (char **)eventPaths, (int)count, rescan);
}

static dispatch_queue_t queue(void) {
	static dispatch_queue_t q;
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		q = dispatch_queue_create("yt-studio.fswatch", DISPATCH_QUEUE_SERIAL);
	});
	return q;
}

// fswatchStartPaths watches roots recursively in one stream, reporting to
// goFSEvents with handle: changed files and folders with fileEvents, or else
// only the folders whose contents changed (far fewer events). Returns NULL
// on failure.
void *fswatchStartPaths(const char **roots, int count, uintptr_t handle, double latency, int fileEvents) {
	CFMutableArrayRef paths = CFArrayCreateMutable(NULL, count, &kCFTypeArrayCallBacks);
	if (paths == NULL) return NULL;
	for (int i = 0; i < count; i++) {
		CFStringRef path = CFStringCreateWithCString(NULL, roots[i], kCFStringEncodingUTF8);
		if (path == NULL) {
			CFRelease(paths);
			return NULL;
		}
		CFArrayAppendValue(paths, path);
		CFRelease(path);
	}
	FSEventStreamContext ctx = {0, (void *)handle, NULL, NULL, NULL};
	FSEventStreamCreateFlags flags = kFSEventStreamCreateFlagNoDefer | kFSEventStreamCreateFlagWatchRoot;
	if (fileEvents) flags |= kFSEventStreamCreateFlagFileEvents;
	FSEventStreamRef stream = FSEventStreamCreate(NULL, onEvents, &ctx, paths, kFSEventStreamEventIdSinceNow, latency, flags);
	CFRelease(paths);
	if (stream == NULL) return NULL;
	FSEventStreamSetDispatchQueue(stream, queue());
	if (!FSEventStreamStart(stream)) {
		FSEventStreamInvalidate(stream);
		FSEventStreamRelease(stream);
		return NULL;
	}
	return (void *)stream;
}

// fswatchStart watches root recursively, reporting changed files and folders
// to goFSEvents with handle. Returns NULL on failure.
void *fswatchStart(const char *root, uintptr_t handle, double latency) {
	return fswatchStartPaths(&root, 1, handle, latency, 1);
}

// fswatchStop stops a stream; no callback runs after it returns.
void fswatchStop(void *stream) {
	FSEventStreamRef s = (FSEventStreamRef)stream;
	FSEventStreamStop(s);
	FSEventStreamInvalidate(s);
	// Wait for callbacks already queued, so the Go handle can be freed.
	dispatch_sync(queue(), ^{});
	FSEventStreamRelease(s);
}
