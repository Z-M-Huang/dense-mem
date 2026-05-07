const DEFAULT_URL = "http://127.0.0.1:8080/mcp";

const FLAG_ALIASES = new Map([
  ["-h", "--help"],
]);

export class ConfigError extends Error {
  constructor(message) {
    super(message);
    this.name = "ConfigError";
  }
}

export function parseConfig(argv = process.argv.slice(2), env = process.env) {
  const parsed = parseArgs(argv);
  if (parsed.help) {
    return {
      help: true,
      helpText: formatHelp(),
    };
  }

  const url = parsed.url ?? env.DENSE_MEM_MCP_URL ?? DEFAULT_URL;
  validateURL(url);

  const headers = {};
  applyHeaders(headers, parseEnvHeaders(env.DENSE_MEM_MCP_HEADERS));
  applyAuth(headers, authFromAPIKey(env.DENSE_MEM_API_KEY));
  applyAuth(headers, env.DENSE_MEM_MCP_AUTHORIZATION);
  applyHeaders(headers, parsed.headers);
  const cliHeaderAuthorization = getHeader(parsed.headers, "Authorization");
  if (!cliHeaderAuthorization) {
    applyAuth(headers, authFromAPIKey(parsed.apiKey));
  }
  applyAuth(headers, cliHeaderAuthorization);
  applyAuth(headers, parsed.authorization);

  if (!getHeader(headers, "Authorization")) {
    throw new ConfigError(
      "Dense-Mem MCP authorization is required. Set DENSE_MEM_API_KEY, DENSE_MEM_MCP_AUTHORIZATION, --api-key, --authorization, or an Authorization --header.",
    );
  }

  return {
    help: false,
    url,
    headers,
    verbose: parsed.verbose,
  };
}

export function parseArgs(argv) {
  const out = {
    headers: {},
    verbose: false,
    help: false,
  };

  for (let i = 0; i < argv.length; i += 1) {
    const raw = argv[i];
    const [flag, inlineValue] = splitFlag(raw);
    const normalizedFlag = FLAG_ALIASES.get(flag) ?? flag;

    switch (normalizedFlag) {
      case "--help":
        out.help = true;
        break;
      case "--verbose":
        out.verbose = true;
        break;
      case "--url":
        out.url = readFlagValue(argv, i, inlineValue, "--url");
        if (inlineValue === undefined) i += 1;
        break;
      case "--api-key":
        out.apiKey = readFlagValue(argv, i, inlineValue, "--api-key");
        if (inlineValue === undefined) i += 1;
        break;
      case "--authorization":
        out.authorization = readFlagValue(argv, i, inlineValue, "--authorization");
        if (inlineValue === undefined) i += 1;
        break;
      case "--header": {
        const header = readFlagValue(argv, i, inlineValue, "--header");
        applyHeaders(out.headers, [parseHeader(header)]);
        if (inlineValue === undefined) i += 1;
        break;
      }
      default:
        throw new ConfigError(`Unknown option: ${raw}`);
    }
  }

  return out;
}

export function parseHeader(rawHeader) {
  if (typeof rawHeader !== "string") {
    throw new ConfigError("Header must be a string");
  }
  rejectLineBreaks(rawHeader, "header");
  const colonIndex = rawHeader.indexOf(":");
  if (colonIndex <= 0) {
    throw new ConfigError(`Invalid header format: ${rawHeader}`);
  }

  const name = rawHeader.slice(0, colonIndex).trim();
  const value = rawHeader.slice(colonIndex + 1).trim();
  validateHeader(name, value);
  return [name, value];
}

export function parseEnvHeaders(rawHeaders) {
  if (!rawHeaders) {
    return [];
  }

  let parsed;
  try {
    parsed = JSON.parse(rawHeaders);
  } catch (error) {
    throw new ConfigError(`DENSE_MEM_MCP_HEADERS must be valid JSON: ${error.message}`);
  }

  if (Array.isArray(parsed)) {
    return parsed.map(parseHeader);
  }

  if (parsed && typeof parsed === "object") {
    return Object.entries(parsed).map(([name, value]) => {
      if (typeof value !== "string") {
        throw new ConfigError(`Header ${name} in DENSE_MEM_MCP_HEADERS must be a string`);
      }
      validateHeader(name, value);
      return [name, value];
    });
  }

  throw new ConfigError("DENSE_MEM_MCP_HEADERS must be a JSON object or array");
}

export function applyHeaders(target, entries) {
  const normalizedEntries = Array.isArray(entries) ? entries : Object.entries(entries);
  for (const [name, value] of normalizedEntries) {
    setHeader(target, name, value);
  }
}

export function applyAuth(target, value) {
  if (!value) {
    return;
  }
  validateHeader("Authorization", value);
  setHeader(target, "Authorization", value);
}

export function authFromAPIKey(apiKey) {
  if (!apiKey) {
    return "";
  }
  rejectLineBreaks(apiKey, "API key");
  const trimmed = apiKey.trim();
  if (!trimmed) {
    throw new ConfigError("API key requires a non-empty value");
  }
  return `Bearer ${trimmed}`;
}

export function formatHelp() {
  return `dense-mem-mcp-proxy

Expose a Dense-Mem Streamable HTTP MCP server as an MCP stdio process.

Usage:
  dense-mem-mcp-proxy [options]

Options:
  --url <url>                 Dense-Mem MCP URL. Defaults to http://127.0.0.1:8080/mcp.
  --api-key <key>             Dense-Mem API key. Builds Authorization: Bearer <key>.
  --authorization <value>     Full Authorization header value.
  --header "Name: value"      Extra HTTP header. Can be repeated.
  --verbose                   Write redacted diagnostic logs to stderr.
  -h, --help                  Show this help.

Environment:
  DENSE_MEM_MCP_URL
  DENSE_MEM_API_KEY
  DENSE_MEM_MCP_AUTHORIZATION
  DENSE_MEM_MCP_HEADERS       JSON object or array of "Name: value" strings.
`;
}

function splitFlag(raw) {
  const equalsIndex = raw.indexOf("=");
  if (equalsIndex === -1) {
    return [raw, undefined];
  }
  return [raw.slice(0, equalsIndex), raw.slice(equalsIndex + 1)];
}

function readFlagValue(argv, index, inlineValue, flag) {
  const value = inlineValue ?? argv[index + 1];
  if (value === undefined || value.startsWith("--")) {
    throw new ConfigError(`${flag} requires a value`);
  }
  if (value.trim() === "") {
    throw new ConfigError(`${flag} requires a non-empty value`);
  }
  rejectLineBreaks(value, flag);
  return value;
}

function validateURL(rawURL) {
  rejectLineBreaks(rawURL, "URL");
  let parsed;
  try {
    parsed = new URL(rawURL);
  } catch {
    throw new ConfigError(`Invalid Dense-Mem MCP URL: ${rawURL}`);
  }
  if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
    throw new ConfigError("Dense-Mem MCP URL must use http or https");
  }
}

function validateHeader(name, value) {
  if (typeof name !== "string" || typeof value !== "string") {
    throw new ConfigError("Header name and value must be strings");
  }
  rejectLineBreaks(name, "header name");
  rejectLineBreaks(value, `header ${name}`);
  if (!name.trim()) {
    throw new ConfigError("Header name is required");
  }
  if (!value.trim()) {
    throw new ConfigError(`Header ${name} requires a non-empty value`);
  }
}

function setHeader(headers, name, value) {
  const existing = Object.keys(headers).find((key) => key.toLowerCase() === name.toLowerCase());
  if (existing && existing !== name) {
    delete headers[existing];
  }
  headers[name] = value;
}

function getHeader(headers, name) {
  const key = Object.keys(headers).find((candidate) => candidate.toLowerCase() === name.toLowerCase());
  return key ? headers[key] : "";
}

function rejectLineBreaks(value, label) {
  if (String(value).includes("\n") || String(value).includes("\r")) {
    throw new ConfigError(`${label} must not contain line breaks`);
  }
}
