# CC Gateway Admin (Vue + Vite)

## Dev

```bash
npm install
npm run dev
```

Default dev URL: `http://127.0.0.1:5173/admin/`  
Vite proxy forwards API calls to `http://127.0.0.1:8080`.

## Build

```bash
npm run build
```

Build output is `web/admin/dist`.

## Backend Integration

Go gateway serves this UI from `/admin/` when `dist/index.html` exists.

- Default dist dir: `web/admin/dist`
- Override via env: `ADMIN_UI_DIST_DIR=/absolute/or/relative/path`

When no built UI exists, gateway falls back to embedded legacy dashboard.
