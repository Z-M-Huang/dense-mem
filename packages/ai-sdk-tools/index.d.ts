import { dynamicTool } from 'ai';

export interface DenseMemToolCatalogEntry {
  name: string;
  description: string;
  input_schema: Record<string, unknown>;
  output_schema: Record<string, unknown>;
  required_scopes: string[];
}

export interface DenseMemToolCatalogResponse {
  tools: DenseMemToolCatalogEntry[];
}

export interface DenseMemToolOptions {
  baseUrl: string;
  apiKey: string;
  fetch?: typeof globalThis.fetch;
  headers?: HeadersInit;
  includeRecallTool?: boolean;
  recallToolName?: string;
}

export interface DenseMemRecallInput {
  query: string;
  limit?: number;
  valid_at?: string | Date;
  known_at?: string | Date;
  include_evidence?: boolean;
}

export interface DenseMemRecallToolConfig {
  name?: string;
  description?: string;
}

export type DenseMemTool = ReturnType<typeof dynamicTool>;
export type DenseMemToolSet = Record<string, DenseMemTool>;

export function createDenseMemTools(options: DenseMemToolOptions): Promise<DenseMemToolSet>;
export function loadDenseMemToolCatalog(options: DenseMemToolOptions): Promise<DenseMemToolCatalogResponse>;
export function createDenseMemCatalogTool(entry: DenseMemToolCatalogEntry, options: DenseMemToolOptions): DenseMemTool;
export function createDenseMemRecallTool(options: DenseMemToolOptions, config?: DenseMemRecallToolConfig): DenseMemTool;
export function executeDenseMemTool(name: string, input: unknown, options: DenseMemToolOptions, abortSignal?: AbortSignal): Promise<unknown>;
export function executeDenseMemRecall(input: DenseMemRecallInput, options: DenseMemToolOptions, abortSignal?: AbortSignal): Promise<unknown>;
