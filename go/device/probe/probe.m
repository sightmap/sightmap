// sightmap device probe: loaded into a simulator app with DYLD_INSERT_LIBRARIES,
// it serves the app's in-process trees over a Unix socket named by
// SIGHTMAP_PROBE_SOCKET. One JSON request per line, one JSON reply per line:
//
//	{"id":1,"cmd":"ping"}
//	{"id":2,"cmd":"dump","args":{"ax":true,"views":false,"layers":false,"swiftui":true,"onScreen":true}}
//
// onScreen leaves out hosting views that are hidden or entirely off screen:
// SwiftUI's debug graph costs 15–100 ms and 0.4–2 MB per host.
//
// The probe only reads; it does no filtering, so it stays small and the rules
// that interpret SwiftUI's private debug data live (and are tested) in Go.
#import <UIKit/UIKit.h>
#import <objc/message.h>
#import <sys/socket.h>
#import <sys/un.h>
#import <unistd.h>
#import <mach/mach_time.h>

static double nowMS(void) {
  static mach_timebase_info_data_t tb;
  if (tb.denom == 0) mach_timebase_info(&tb);
  return (double)mach_absolute_time() * tb.numer / tb.denom / 1e6;
}

static NSArray *rect(CGRect f) {
  return @[ @(round(f.origin.x * 10) / 10), @(round(f.origin.y * 10) / 10), @(round(f.size.width * 10) / 10), @(round(f.size.height * 10) / 10) ];
}

static NSString *ptr(id o) { return [NSString stringWithFormat:@"%p", o]; }

static SEL debugSel(void) { return NSSelectorFromString(@"makeViewDebugData"); }

static NSArray<UIWindow *> *windows(void) {
  NSMutableArray *out = [NSMutableArray array];
  for (UIScene *sc in UIApplication.sharedApplication.connectedScenes) {
    if (![sc isKindOfClass:[UIWindowScene class]]) continue;
    for (UIWindow *w in ((UIWindowScene *)sc).windows)
      if (!w.hidden) [out addObject:w];
  }
  [out sortUsingComparator:^NSComparisonResult(UIWindow *a, UIWindow *b) { return [@(a.windowLevel) compare:@(b.windowLevel)]; }];
  return out;
}

static BOOL visible(UIView *v) {
  for (; v; v = v.superview)
    if (v.hidden || v.alpha < 0.01) return NO;
  return YES;
}

static NSDictionary *walkView(UIView *v, NSMutableArray *hosts) {
  NSMutableDictionary *n = [@{@"cls" : NSStringFromClass([v class]), @"ptr" : ptr(v), @"f" : rect([v convertRect:v.bounds toView:nil])} mutableCopy];
  if (v.accessibilityIdentifier.length) n[@"id"] = v.accessibilityIdentifier;
  if (v.hidden) n[@"hidden"] = @YES;
  if ([v respondsToSelector:debugSel()]) [hosts addObject:v];
  NSMutableArray *kids = [NSMutableArray array];
  for (UIView *s in v.subviews) [kids addObject:walkView(s, hosts)];
  if (kids.count) n[@"kids"] = kids;
  return n;
}

static NSDictionary *walkLayer(CALayer *l, CALayer *root) {
  NSMutableDictionary *n = [@{@"cls" : NSStringFromClass([l class]), @"f" : rect([l convertRect:l.bounds toLayer:root])} mutableCopy];
  if (l.delegate) n[@"delegate"] = NSStringFromClass([(NSObject *)l.delegate class]);
  NSMutableArray *kids = [NSMutableArray array];
  for (CALayer *s in l.sublayers) [kids addObject:walkLayer(s, root)];
  if (kids.count) n[@"kids"] = kids;
  return n;
}

static NSArray *axChildren(id e) {
  NSArray *kids = [e respondsToSelector:@selector(accessibilityElements)] ? [e accessibilityElements] : nil;
  if (!kids) {
    NSInteger c = [e accessibilityElementCount];
    if (c != NSNotFound && c > 0) {
      NSMutableArray *a = [NSMutableArray array];
      for (NSInteger i = 0; i < c; i++) {
        id k = [e accessibilityElementAtIndex:i];
        if (k) [a addObject:k];
      }
      kids = a;
    }
  }
  if (!kids && [e isKindOfClass:[UIView class]]) kids = [(UIView *)e subviews];
  return kids;
}

static NSDictionary *walkAX(id e, int depth) {
  NSMutableDictionary *n = [@{@"cls" : NSStringFromClass([e class]), @"f" : rect([e accessibilityFrame])} mutableCopy];
  if ([e isAccessibilityElement]) n[@"el"] = @YES;
  NSString *s = [e respondsToSelector:@selector(accessibilityIdentifier)] ? [e accessibilityIdentifier] : nil;
  if (s.length) n[@"id"] = s;
  if ((s = [e accessibilityLabel]).length) n[@"label"] = s;
  if ((s = [e accessibilityValue]).length) n[@"value"] = s;
  if ([e accessibilityTraits]) n[@"traits"] = @([e accessibilityTraits]);
  if ([e isKindOfClass:[UIView class]]) n[@"ptr"] = ptr(e);
  if (depth < 60) {
    NSMutableArray *kids = [NSMutableArray array];
    for (id k in axChildren(e)) [kids addObject:walkAX(k, depth + 1)];
    if (kids.count) n[@"kids"] = kids;
  }
  return n;
}

static NSData *json(id obj) { return [NSJSONSerialization dataWithJSONObject:obj options:0 error:nil] ?: [@"null" dataUsingEncoding:NSUTF8StringEncoding]; }

static void append(NSMutableData *d, NSString *s) { [d appendData:[s dataUsingEncoding:NSUTF8StringEncoding]]; }

// dump runs on the main thread. The reply is assembled by hand so each host's
// debug data, already JSON, is spliced in without a parse and re-encode.
static NSData *dump(NSDictionary *args, NSNumber *rid) {
  BOOL wantAX = [args[@"ax"] boolValue], wantViews = [args[@"views"] boolValue], wantLayers = [args[@"layers"] boolValue], wantUI = [args[@"swiftui"] boolValue],
       onScreen = [args[@"onScreen"] boolValue];
  NSInteger skipped = 0;
  NSMutableDictionary *meta = [NSMutableDictionary dictionary], *timing = [NSMutableDictionary dictionary];
  double t0 = nowMS();
  NSArray<UIWindow *> *ws = windows();
  NSMutableArray *hosts = [NSMutableArray array], *viewTrees = [NSMutableArray array];
  for (UIWindow *w in ws) [viewTrees addObject:walkView(w, hosts)];
  timing[@"views"] = @(nowMS() - t0);
  if (wantViews) meta[@"views"] = viewTrees;
  if (wantLayers) {
    double t = nowMS();
    NSMutableArray *ls = [NSMutableArray array];
    for (UIWindow *w in ws) [ls addObject:walkLayer(w.layer, w.layer)];
    meta[@"layers"] = ls;
    timing[@"layers"] = @(nowMS() - t);
  }
  if (wantAX) {
    double t = nowMS();
    NSMutableArray *ax = [NSMutableArray array];
    for (UIWindow *w in ws) [ax addObject:walkAX(w, 0)];
    meta[@"ax"] = ax;
    timing[@"ax"] = @(nowMS() - t);
  }
  UIScreen *screen = ws.firstObject.screen ?: UIScreen.mainScreen;
  meta[@"screen"] = @{@"w" : @(screen.bounds.size.width), @"h" : @(screen.bounds.size.height)};
  NSMutableArray *graphs = [NSMutableArray array], *hostInfo = [NSMutableArray array];
  if (wantUI) {
    for (UIView *h in hosts) {
      if (!h.window) continue;
      CGRect f = [h convertRect:h.bounds toCoordinateSpace:screen.coordinateSpace];
      if (onScreen && (!visible(h) || !CGRectIntersectsRect(f, screen.bounds))) {
        skipped++;
        continue;
      }
      double t = nowMS();
      NSData *d = ((NSData * (*)(id, SEL)) objc_msgSend)(h, debugSel());
      double ms = nowMS() - t;
      [hostInfo addObject:@{
        @"cls" : NSStringFromClass([h class]),
        @"ptr" : ptr(h),
        @"f" : rect(f),
        @"ms" : @(ms),
        @"bytes" : @(d.length)
      }];
      [graphs addObject:d.length ? d : [@"[]" dataUsingEncoding:NSUTF8StringEncoding]];
    }
  }
  timing[@"total"] = @(nowMS() - t0);
  meta[@"timing"] = timing;
  meta[@"skipped"] = @(skipped);
  meta[@"swiftuiDebug"] = NSProcessInfo.processInfo.environment[@"SWIFTUI_VIEW_DEBUG"] ?: @"";

  NSMutableData *out = [NSMutableData data];
  append(out, [NSString stringWithFormat:@"{\"id\":%@,\"ok\":true,\"result\":{\"meta\":", rid ?: @0]);
  [out appendData:json(meta)];
  append(out, @",\"hosts\":[");
  for (NSUInteger i = 0; i < hostInfo.count; i++) {
    if (i) append(out, @",");
    append(out, @"{\"info\":");
    [out appendData:json(hostInfo[i])];
    append(out, @",\"graph\":");
    [out appendData:graphs[i]];
    append(out, @"}");
  }
  append(out, @"]}}");
  return out;
}

static NSData *handle(NSData *line) {
  NSDictionary *req = [NSJSONSerialization JSONObjectWithData:line options:0 error:nil];
  if (![req isKindOfClass:[NSDictionary class]]) return json(@{@"ok" : @NO, @"error" : @{@"code" : @"bad_request", @"message" : @"unparseable request"}});
  NSString *cmd = req[@"cmd"];
  NSNumber *rid = req[@"id"];
  NSDictionary *args = [req[@"args"] isKindOfClass:[NSDictionary class]] ? req[@"args"] : @{};
  if ([cmd isEqual:@"ping"]) {
    return json(@{
      @"id" : rid ?: @0,
      @"ok" : @YES,
      @"result" : @{@"pid" : @(getpid()), @"bundleId" : NSBundle.mainBundle.bundleIdentifier ?: @"", @"swiftuiDebug" : NSProcessInfo.processInfo.environment[@"SWIFTUI_VIEW_DEBUG"] ?: @""}
    });
  }
  if ([cmd isEqual:@"dump"]) {
    __block NSData *out;
    dispatch_sync(dispatch_get_main_queue(), ^{
      @autoreleasepool {
        out = dump(args, rid);
      }
    });
    return out;
  }
  return json(@{@"id" : rid ?: @0, @"ok" : @NO, @"error" : @{@"code" : @"unknown_command", @"message" : cmd ?: @""}});
}

static void serve(int fd) {
  NSMutableData *buf = [NSMutableData data];
  char chunk[65536];
  for (;;) {
    ssize_t n = read(fd, chunk, sizeof chunk);
    if (n <= 0) break;
    [buf appendBytes:chunk length:n];
    for (;;) {
      const char *b = buf.bytes;
      const char *nl = memchr(b, '\n', buf.length);
      if (!nl) break;
      NSUInteger len = nl - b;
      NSData *reply;
      @autoreleasepool {
        reply = handle([buf subdataWithRange:NSMakeRange(0, len)]);
      }
      [buf replaceBytesInRange:NSMakeRange(0, len + 1) withBytes:NULL length:0];
      const char *p = reply.bytes;
      size_t left = reply.length;
      while (left > 0) {
        ssize_t w = write(fd, p, left);
        if (w <= 0) goto done;
        p += w, left -= w;
      }
      if (write(fd, "\n", 1) != 1) goto done;
    }
  }
done:
  close(fd);
}

__attribute__((constructor)) static void start(void) {
  const char *path = getenv("SIGHTMAP_PROBE_SOCKET");
  if (!path || !*path) return;
  struct sockaddr_un addr = {.sun_family = AF_UNIX};
  if (strlen(path) >= sizeof addr.sun_path) return;
  strcpy(addr.sun_path, path);
  unlink(path);
  int lfd = socket(AF_UNIX, SOCK_STREAM, 0);
  if (lfd < 0 || bind(lfd, (struct sockaddr *)&addr, sizeof addr) != 0 || listen(lfd, 4) != 0) return;
  // Children the app spawns inherit the environment; only the first process
  // owns the socket.
  unsetenv("SIGHTMAP_PROBE_SOCKET");
  unsetenv("DYLD_INSERT_LIBRARIES");
  dispatch_async(dispatch_get_global_queue(QOS_CLASS_USER_INITIATED, 0), ^{
    for (;;) {
      int fd = accept(lfd, NULL, NULL);
      if (fd < 0) continue;
      dispatch_async(dispatch_get_global_queue(QOS_CLASS_USER_INITIATED, 0), ^{ serve(fd); });
    }
  });
}
