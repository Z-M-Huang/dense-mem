const SENSITIVE_KEY = /authorization|api[-_]?key|token|secret/i;
const BEARER_VALUE = /Bearer\s+[A-Za-z0-9._~+/=-]+/gi;
const DENSE_MEM_KEY = /dm_live_[A-Za-z0-9_-]+/g;

export function createLogger({ verbose = false, stderr = process.stderr } = {}) {
  return {
    info: (...args) => {
      if (verbose) {
        write(stderr, args);
      }
    },
    error: (...args) => {
      write(stderr, args);
    },
  };
}

export function redact(value) {
  if (value instanceof Error) {
    return redact(value.message);
  }
  if (Array.isArray(value)) {
    return value.map(redact);
  }
  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.entries(value).map(([key, item]) => [
        key,
        SENSITIVE_KEY.test(key) ? "[redacted]" : redact(item),
      ]),
    );
  }
  if (typeof value === "string") {
    return value.replace(BEARER_VALUE, "Bearer [redacted]").replace(DENSE_MEM_KEY, "dm_live_[redacted]");
  }
  return value;
}

function write(stderr, args) {
  const line = args.map(format).join(" ");
  stderr.write(`${line}\n`);
}

function format(value) {
  const redacted = redact(value);
  if (typeof redacted === "string") {
    return redacted;
  }
  return JSON.stringify(redacted);
}
