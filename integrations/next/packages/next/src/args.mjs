// Tiny flag parser: --k v, --k=v, --flag, --no-flag. Positionals in `_`.
export function parseArgs(argv, { booleans = [] } = {}) {
  const out = { _: [] };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (!a.startsWith("--")) {
      out._.push(a);
      continue;
    }
    const eq = a.indexOf("=");
    if (eq !== -1) {
      out[a.slice(2, eq)] = a.slice(eq + 1);
      continue;
    }
    const key = a.slice(2);
    if (key.startsWith("no-")) {
      out[key.slice(3)] = false;
      continue;
    }
    if (
      booleans.includes(key) ||
      i + 1 >= argv.length ||
      argv[i + 1].startsWith("--")
    ) {
      out[key] = true;
      continue;
    }
    out[key] = argv[++i];
  }
  return out;
}
