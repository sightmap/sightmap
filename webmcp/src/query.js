// Component-query parser — a compatible subset of the sightmap CLI's
// component-query DSL (see the sightmap-browser skill):
//
//   ProductCard                          bare component name
//   ProductCard[name^="Weber"]           predicate on an extracted property
//   ProductCard[name*="weber" i]         substring, case-insensitive
//   LoginForm UserNameInput              descendant chain (target = last)
//   FulfillmentTileButton#1              occurrence index (0-based)
//   css:.plp-price-module a              raw-CSS escape hatch (not in the CLI)
//
// Operators: `=` exact, `^=` prefix, `*=` substring; trailing ` i` flag for
// case-insensitive. Whitespace is a descendant combinator; there is no `>`.
// Predicate values may carry `{param}` templates, interpolated at runtime.

const NAME_RE = /^[A-Za-z][A-Za-z0-9_-]*/;

function parseLink(text) {
  const nameMatch = text.match(NAME_RE);
  if (!nameMatch) throw new Error(`expected a component name at "${text}"`);
  const link = { name: nameMatch[0], preds: [], index: null };
  let rest = text.slice(nameMatch[0].length);
  while (rest.startsWith("[")) {
    const end = findPredEnd(rest);
    link.preds.push(parsePred(rest.slice(1, end)));
    rest = rest.slice(end + 1);
  }
  if (rest.startsWith("#")) {
    const m = rest.match(/^#(\d+)/);
    if (!m) throw new Error(`expected an index after "#" in "${text}"`);
    link.index = parseInt(m[1], 10);
    rest = rest.slice(m[0].length);
  }
  if (rest.length > 0) {
    throw new Error(`unexpected trailing "${rest}" in query link "${text}"`);
  }
  return link;
}

function findPredEnd(s) {
  // s starts with '['; find the matching ']' respecting quotes.
  let quote = null;
  for (let i = 1; i < s.length; i++) {
    const c = s[i];
    if (quote) {
      if (c === quote) quote = null;
    } else if (c === '"' || c === "'") {
      quote = c;
    } else if (c === "]") {
      return i;
    }
  }
  throw new Error(`unterminated predicate in "${s}"`);
}

function parsePred(body) {
  const m = body.match(/^([a-z][a-z0-9_]*)\s*(\^=|\*=|=)\s*([\s\S]*)$/);
  if (!m) throw new Error(`cannot parse predicate "[${body}]"`);
  let value = m[3].trim();
  let ci = false;
  // A trailing ` i` flag counts when what precedes it is a complete quoted
  // string, or when the whole value is bare (an unquoted value can't itself
  // end in ` i` and mean the letter).
  const noFlag = value.replace(/\s+i$/, "");
  if (noFlag !== value && (isQuoted(noFlag) || !isQuoted(value))) {
    ci = true;
    value = noFlag;
  }
  if (isQuoted(value)) value = value.slice(1, -1);
  return { prop: m[1], op: m[2], value, ci };
}

function isQuoted(v) {
  return (
    (v.startsWith('"') && v.endsWith('"') && v.length >= 2) ||
    (v.startsWith("'") && v.endsWith("'") && v.length >= 2)
  );
}

// splitLinks splits on whitespace that is outside [...] predicates.
function splitLinks(q) {
  const parts = [];
  let depth = 0;
  let quote = null;
  let cur = "";
  for (const c of q) {
    if (quote) {
      cur += c;
      if (c === quote) quote = null;
    } else if (c === '"' || c === "'") {
      cur += c;
      quote = c;
    } else if (c === "[") {
      depth++;
      cur += c;
    } else if (c === "]") {
      depth--;
      cur += c;
    } else if (/\s/.test(c) && depth === 0) {
      if (cur) parts.push(cur);
      cur = "";
    } else {
      cur += c;
    }
  }
  if (cur) parts.push(cur);
  return parts;
}

// parseQuery parses a component query (or css: escape) into an IR the
// compiler resolves against the corpus.
function parseQuery(q) {
  const text = String(q).trim();
  if (text.startsWith("css:")) {
    const selector = text.slice(4).trim();
    if (!selector) throw new Error(`empty selector in "${q}"`);
    return { kind: "css", selector };
  }
  if (!text) throw new Error("empty component query");
  if (/\s>\s|>/.test(text.replace(/\[[^\]]*\]/g, ""))) {
    throw new Error(
      `"${q}": the child combinator ">" is not supported — whitespace is a descendant combinator`,
    );
  }
  const links = splitLinks(text).map(parseLink);
  if (links.length === 0) throw new Error("empty component query");
  return { kind: "query", links };
}

module.exports = { parseQuery };
