package main

/*
#cgo LDFLAGS: -framework CoreGraphics -framework CoreFoundation
#include <CoreGraphics/CoreGraphics.h>
#include <CoreFoundation/CoreFoundation.h>

void probe() {
    CFArrayRef list = CGWindowListCopyWindowInfo(kCGWindowListOptionAll, kCGNullWindowID);
    if (list == NULL) return;
    CFIndex count = CFArrayGetCount(list);
    for (CFIndex i = 0; i < count; i++) {
        CFDictionaryRef info = (CFDictionaryRef)CFArrayGetValueAtIndex(list, i);
        CFNumberRef pidRef = (CFNumberRef)CFDictionaryGetValue(info, kCGWindowOwnerPID);
        int pid = 0;
        CFNumberGetValue(pidRef, kCFNumberIntType, &pid);
        
        if (pid != 22728) continue;

        CFStringRef owner = (CFStringRef)CFDictionaryGetValue(info, kCGWindowOwnerName);
        char ownerBuf[256] = {0};
        if (owner) CFStringGetCString(owner, ownerBuf, 256, kCFStringEncodingUTF8);
        
        CFStringRef title = (CFStringRef)CFDictionaryGetValue(info, kCGWindowName);
        char titleBuf[256] = {0};
        if (title) CFStringGetCString(title, titleBuf, 256, kCFStringEncodingUTF8);

        CFDictionaryRef bounds = (CFDictionaryRef)CFDictionaryGetValue(info, kCGWindowBounds);
        CGRect rect;
        CGRectMakeWithDictionaryRepresentation(bounds, &rect);

        printf("PID: %d | Owner: %s | Title: %s | Bounds: %.0f, %.0f, %.0f x %.0f\n", 
            pid, ownerBuf, titleBuf, rect.origin.x, rect.origin.y, rect.size.width, rect.size.height);
    }
    CFRelease(list);
}
*/
import "C"

func main() {
	C.probe()
}
