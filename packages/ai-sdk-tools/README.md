# `@dense-mem/ai-sdk-tools`

HTTP-backed Vercel AI SDK tools for dense-mem.

This package is the AI-facing tool adapter for dense-mem. It discovers the live
tool catalog from the server and exposes a first-class `recall_knowledge` tool
for high-level memory retrieval.

## What It Exposes

- All tools from `GET /api/v1/tools`
- An explicit `recall_knowledge` tool over `GET /api/v1/recall`

This package is intentionally tools-only. It does not try to be a general TypeScript client.

## Install

```bash
npm install ai @dense-mem/ai-sdk-tools
```

## Usage

```ts
import { generateText } from 'ai';
import { openai } from '@ai-sdk/openai';
import { createDenseMemTools } from '@dense-mem/ai-sdk-tools';

const tools = await createDenseMemTools({
  baseUrl: process.env.DENSE_MEM_URL!,
  apiKey: process.env.DENSE_MEM_API_KEY!,
});

const result = await generateText({
  model: openai('gpt-5.4'),
  tools,
  prompt: 'Recall the most relevant memories about my travel plans.',
});
```

## Options

`createDenseMemTools(options)` accepts:

- `baseUrl`: dense-mem base URL, for example `https://dense-mem.example.com`
- `apiKey`: dense-mem bearer token
- `fetch`: optional custom `fetch` implementation
- `headers`: optional extra headers added to every request
- `includeRecallTool`: disable the default `recall_knowledge` tool when set to `false`
- `recallToolName`: rename `recall_knowledge` if your app wants a different tool id

## Recall Input

The default `recall_knowledge` tool accepts:

- `query`: natural-language memory query
- `limit`: result limit, between `1` and `50`
- `valid_at`: optional temporal filter for when information was true
- `known_at`: optional temporal filter for when information was known
- `include_evidence`: include fragment evidence in the recall response

## Behavior

- `recall_knowledge` is included by default because it is the preferred high-level retrieval surface.
- Catalog tools keep their server-side names, including hyphenated ones such as `keyword-search`.
- Catalog discovery does not hide tools based on deployment state; execution errors come back from the server.
- The package is intentionally tools-only. It does not try to be a general TypeScript client.

## Publish Checks

Before publishing, run:

```bash
cd packages/ai-sdk-tools
npm run check
npm run pack:dry-run
```

The repo also includes a GitHub Actions workflow for manual or tag-based npm publishing.

## Notes

- Node `22+` is required.
- The package declares `ai >=6.0.0` as a peer dependency.
