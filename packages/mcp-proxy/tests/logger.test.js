import assert from "node:assert/strict";
import test from "node:test";

import { createLogger, redact } from "../src/logger.js";

test("redact removes bearer tokens and Dense-Mem API keys from strings", () => {
  assert.equal(
    redact("Authorization: Bearer dm_live_secret and dm_live_anothersecret"),
    "Authorization: Bearer [redacted] and dm_live_[redacted]",
  );
});

test("redact removes sensitive object fields", () => {
  assert.deepEqual(redact({ Authorization: "Bearer secret", other: "visible" }), {
    Authorization: "[redacted]",
    other: "visible",
  });
});

test("logger writes info only when verbose", () => {
  const quiet = captureLogger(false);
  quiet.logger.info("Authorization: Bearer dm_live_secret");
  assert.equal(quiet.output(), "");

  const verbose = captureLogger(true);
  verbose.logger.info("Authorization: Bearer dm_live_secret");
  assert.match(verbose.output(), /Bearer \[redacted\]/);
  assert.doesNotMatch(verbose.output(), /dm_live_secret/);
});

test("logger always writes redacted errors", () => {
  const captured = captureLogger(false);
  captured.logger.error("failed", { Authorization: "Bearer dm_live_secret" });

  assert.match(captured.output(), /failed/);
  assert.match(captured.output(), /\[redacted\]/);
  assert.doesNotMatch(captured.output(), /dm_live_secret/);
});

function captureLogger(verbose) {
  let buffer = "";
  const stderr = {
    write(chunk) {
      buffer += chunk;
    },
  };
  return {
    logger: createLogger({ verbose, stderr }),
    output: () => buffer,
  };
}
