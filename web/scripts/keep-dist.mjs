// Restores dist/.gitkeep after a build.
//
// vite builds with emptyOutDir, which wipes dist/ — including the committed
// placeholder that keeps `//go:embed all:dist` (web/embed.go) compiling on a
// checkout with no built frontend. Without it, every Go job fails with
// "pattern all:dist: no matching files found" while the Docker image build
// stays green (it builds the SPA first), so the breakage is easy to miss.
//
// That has happened twice (fixed in 3f7c267, deleted again by c248ba4, which
// left main red for two days) — both times because someone ran a local build
// and committed the deletion. Recreating it here removes the footgun.
import { writeFileSync, mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const dist = join(dirname(dirname(fileURLToPath(import.meta.url))), "dist");
const body = `# Keeps the dist/ directory present so \`//go:embed all:dist\` (web/embed.go)
# compiles before the frontend is built. The real build output (gitignored)
# replaces this at image-build time. Do not delete.
`;

mkdirSync(dist, { recursive: true });
writeFileSync(join(dist, ".gitkeep"), body);
