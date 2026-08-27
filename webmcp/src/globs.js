// Route-glob helpers, implementing the spec's "Route matching" rules:
//   - `*` matches exactly one path segment
//   - `**` as its own segment matches zero or more segments
//   - `**` glued into a segment degrades to an in-segment `*`
//   - Express-style `:param` segments normalize to `*`
//   - matching is case-sensitive, against the decoded pathname, query and
//     fragment ignored, trailing slashes normalized away

function escapeRegex(s) {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function normalizePath(p) {
  let out = p || "/";
  out = out.split("?")[0].split("#")[0];
  if (out.length > 1) out = out.replace(/\/+$/, "");
  if (!out.startsWith("/")) out = "/" + out;
  return out;
}

function segmentToRegex(seg) {
  if (seg === "*" || seg.startsWith(":")) return "[^/]+";
  // In-segment wildcards: `**` glued into a segment is a regular `*`.
  const collapsed = seg.replace(/\*{2,}/g, "*");
  return collapsed.split("*").map(escapeRegex).join("[^/]*");
}

// routeGlobToRegex compiles a sightmap route glob to a RegExp over a
// normalized pathname.
function routeGlobToRegex(route) {
  const norm = normalizePath(String(route));
  if (norm === "/") return new RegExp("^/$");
  const segs = norm.split("/").filter((s) => s.length > 0);
  const parts = [];
  for (const seg of segs) {
    if (seg === "**") {
      // Zero or more whole segments (including none).
      parts.push("(?:/[^/]+)*");
    } else {
      parts.push("/" + segmentToRegex(seg));
    }
  }
  let src = "^" + parts.join("") + "$";
  // A glob whose segments can all match zero segments (e.g. "/**") must also
  // match the normalized root path "/".
  if (new RegExp(src).test("")) src = "^(?:" + parts.join("") + "|/)$";
  return new RegExp(src);
}

// pathMatchesRoute tests a concrete pathname against a route glob.
function pathMatchesRoute(pathname, route) {
  return routeGlobToRegex(route).test(normalizePath(pathname));
}

module.exports = { routeGlobToRegex, pathMatchesRoute, normalizePath };
