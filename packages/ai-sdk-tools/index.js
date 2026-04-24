import { dynamicTool, jsonSchema } from 'ai';

const recallKnowledgeSchema = {
  type: 'object',
  required: ['query'],
  properties: {
    query: {
      type: 'string',
      maxLength: 512,
      description: 'Natural-language recall query.'
    },
    limit: {
      type: 'integer',
      minimum: 1,
      maximum: 50
    },
    valid_at: {
      type: 'string',
      format: 'date-time'
    },
    known_at: {
      type: 'string',
      format: 'date-time'
    },
    include_evidence: {
      type: 'boolean'
    }
  },
  additionalProperties: false
};

const recallKnowledgeDescription =
  'High-level dense-mem recall across facts, validated claims, and fragments. Use this as the primary memory retrieval tool.';

export async function createDenseMemTools(options) {
  const catalog = await loadDenseMemToolCatalog(options);
  const tools = {};

  for (const entry of catalog.tools) {
    tools[entry.name] = createDenseMemCatalogTool(entry, options);
  }

  if (options.includeRecallTool !== false) {
    const recallToolName = options.recallToolName || 'recall_knowledge';
    tools[recallToolName] = createDenseMemRecallTool(options, { name: recallToolName });
  }

  return tools;
}

export async function loadDenseMemToolCatalog(options) {
  assertBaseOptions(options);

  return requestJson({
    options,
    method: 'GET',
    path: '/api/v1/tools'
  });
}

export function createDenseMemCatalogTool(entry, options) {
  assertBaseOptions(options);

  return dynamicTool({
    description: entry.description,
    inputSchema: jsonSchema(entry.input_schema || { type: 'object' }),
    execute: async (input, executionOptions = {}) =>
      executeDenseMemTool(entry.name, input, options, executionOptions.abortSignal)
  });
}

export function createDenseMemRecallTool(options, config = {}) {
  assertBaseOptions(options);

  return dynamicTool({
    description: config.description || recallKnowledgeDescription,
    inputSchema: jsonSchema(recallKnowledgeSchema),
    execute: async (input, executionOptions = {}) =>
      executeDenseMemRecall(input, options, executionOptions.abortSignal)
  });
}

export async function executeDenseMemTool(name, input, options, abortSignal) {
  assertBaseOptions(options);

  return requestJson({
    options,
    method: 'POST',
    path: `/api/v1/tools/${encodeURIComponent(name)}`,
    body: input || {},
    signal: abortSignal
  });
}

export async function executeDenseMemRecall(input, options, abortSignal) {
  assertBaseOptions(options);

  const params = new URLSearchParams();
  params.set('query', String(input?.query || ''));

  if (typeof input?.limit === 'number') {
    params.set('limit', String(input.limit));
  }
  if (input?.valid_at) {
    params.set('valid_at', toDateTimeString(input.valid_at));
  }
  if (input?.known_at) {
    params.set('known_at', toDateTimeString(input.known_at));
  }
  if (input?.include_evidence === true) {
    params.set('include_evidence', 'true');
  }

  return requestJson({
    options,
    method: 'GET',
    path: `/api/v1/recall?${params.toString()}`,
    signal: abortSignal
  });
}

async function requestJson({ options, method, path, body, signal }) {
  const fetchImpl = options.fetch || globalThis.fetch;
  if (typeof fetchImpl !== 'function') {
    throw new Error('dense-mem ai-sdk-tools: fetch is not available');
  }

  const response = await fetchImpl(`${normalizeBaseUrl(options.baseUrl)}${path}`, {
    method,
    headers: buildHeaders(options.headers, options.apiKey, body !== undefined),
    body: body === undefined ? undefined : JSON.stringify(body),
    signal
  });

  if (!response.ok) {
    const errorBody = await response.text();
    throw new Error(
      `dense-mem request failed (${response.status} ${response.statusText}): ${compactErrorBody(errorBody)}`
    );
  }

  if (response.status === 204) {
    return undefined;
  }

  return response.json();
}

function buildHeaders(extraHeaders, apiKey, hasJsonBody) {
  const headers = new Headers(extraHeaders || {});
  headers.set('Authorization', `Bearer ${apiKey}`);

  if (hasJsonBody) {
    headers.set('Content-Type', 'application/json');
  }

  return headers;
}

function assertBaseOptions(options) {
  if (!options || typeof options !== 'object') {
    throw new Error('dense-mem ai-sdk-tools: options are required');
  }
  if (!options.baseUrl) {
    throw new Error('dense-mem ai-sdk-tools: baseUrl is required');
  }
  if (!options.apiKey) {
    throw new Error('dense-mem ai-sdk-tools: apiKey is required');
  }
}

function normalizeBaseUrl(baseUrl) {
  return String(baseUrl).replace(/\/+$/, '');
}

function toDateTimeString(value) {
  if (value instanceof Date) {
    return value.toISOString();
  }
  return String(value);
}

function compactErrorBody(value) {
  const body = String(value || '').replace(/\s+/g, ' ').trim();
  return body.slice(0, 400);
}
