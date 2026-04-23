# npm Publish

This repo publishes the AI SDK adapter as `@dense-mem/ai-sdk-tools`.

## Release Checklist

1. Update the version in `packages/ai-sdk-tools/package.json`.
2. Review the consumer-facing package README in `packages/ai-sdk-tools/README.md`.
3. Run the local publish checks:

```bash
cd packages/ai-sdk-tools
npm run check
npm run pack:dry-run
```

4. Publish through GitHub Actions or from a local shell with npm credentials.

## GitHub Actions Publish

The workflow at `.github/workflows/publish-ai-sdk-tools.yml` supports:

- `workflow_dispatch` for an explicit manual publish
- `push` on tags matching `ai-sdk-tools-v*`

The workflow expects the repository secret `NPM_TOKEN` and publishes with npm
provenance enabled.

## Local Publish

```bash
cd packages/ai-sdk-tools
npm publish --access public
```

`publishConfig.access` is already set to `public`, so the explicit flag is only
needed if you want to be defensive in local release commands.
