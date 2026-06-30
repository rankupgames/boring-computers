# boring-computers

A [Turborepo](https://turbo.build/repo) monorepo (npm workspaces).

## Structure

```
apps/
  web/                    SvelteKit app (Geist design system) — the hero site
packages/
  eslint-config/          @boring/eslint-config   — shared ESLint flat config (Svelte)
  prettier-config/        @boring/prettier-config — shared Prettier options + plugins
  typescript-config/      @boring/typescript-config — shared tsconfig bases
```

## Getting started

```sh
npm install        # installs all workspaces
npm run dev        # turbo run dev   — starts apps/web
```

## Tasks

All tasks are orchestrated by Turborepo from the repo root:

```sh
npm run dev        # dev servers (persistent)
npm run build      # production builds
npm run check      # svelte-check / type-check
npm run lint       # prettier --check + eslint
npm run format     # prettier --write
npm run test       # unit + e2e
```

Run a task for a single workspace with a filter, e.g. `npx turbo run build --filter=web`,
or work inside an app directly: `npm run dev -w web`.
