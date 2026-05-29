# COMRAD Dashboard Frontend

This is the Manager dashboard source. It is a Vite React app using shadcn/ui components.

## Commands

```sh
npm ci
npm run build
```

The build output goes to `../../internal/comrad/dashboard_static` and is embedded into the Manager binary. The repo-level `make validate` and `make build` run this build automatically.

For local dashboard development, run a Manager on `127.0.0.1:1922`; the Vite dev server proxies `/api/*` to that Manager so `/dashboard/` can use the same admin API paths as the embedded dashboard.

Add shadcn components from this directory:

```sh
npx shadcn@latest add <component>
```
