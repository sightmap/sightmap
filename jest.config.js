// Tests the browser-side JS embedded (via go:embed) into the Go binary —
// go/**/*.js files evaluated in jsdom, next to a *.test.js sibling.
module.exports = {
  testEnvironment: "jsdom",
  testMatch: ["<rootDir>/go/**/*.test.js"],
  testPathIgnorePatterns: ["/node_modules/", "<rootDir>/go/npm/"],
  // This machine's system-wide watchman can't write its LaunchAgents plist
  // under sandboxing, which crashes Jest's file crawl. Not needed for a
  // one-shot `jest` run anyway.
  watchman: false,
};
