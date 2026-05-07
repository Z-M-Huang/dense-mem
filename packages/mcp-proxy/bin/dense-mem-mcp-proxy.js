#!/usr/bin/env node

import { run } from "../src/run.js";

const code = await run();
if (code !== 0) {
  process.exitCode = code;
}
