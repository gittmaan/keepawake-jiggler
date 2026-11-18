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
        
        CFStringRef owner = (CFStringRef)CFDictionaryGetValue(info, kCGWindowOwnerName);
        char ownerBuf[256] = {0};
        if (owner) CFStringGetCString(owner, ownerBuf, 256, kCFStringEncodingUTF8);
        
        CFStringRef title = (CFStringRef)CFDictionaryGetValue(info, kCGWindowName);
        char titleBuf[256] = {0};
        if (title) CFStringGetCString(title, titleBuf, 256, kCFStringEncodingUTF8);

        CFNumberRef layerRef = (CFNumberRef)CFDictionaryGetValue(info, kCGWindowLayer);
        int layer = 0;
        CFNumberGetValue(layerRef, kCFNumberIntType, &layer);

        if (pid == 22728 || (owner && (strstr(ownerBuf, "Citrix") || strstr(ownerBuf, "Viewer")))) {
            printf("PID: %d | Owner: %s | Title: %s | Layer: %d\n", pid, ownerBuf, titleBuf, layer);
        }
    }
    CFRelease(list);
}
*/
import "C"

func main() {
	C.probe()
}
