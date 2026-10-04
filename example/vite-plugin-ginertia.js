// Vite plugin for ginertia (Go). Mirrors what laravel-vite-plugin does:
//  - dev:   writes the dev server URL to public/hot (removed on exit)
//  - build: manifest in public/build, base URL /build/
//  - ssr:   `vite build --ssr` outputs bootstrap/ssr/ssr.js
import fs from 'node:fs'
import path from 'node:path'

export default function ginertia({
  input = 'frontend/app.js',
  ssr,
  publicDir = 'public',
  buildDir = 'build',
  hotFile,
  ssrOutDir = 'bootstrap/ssr',
} = {}) {
  const hot = hotFile ?? path.join(publicDir, 'hot')

  return {
    name: 'ginertia',
    enforce: 'post',

    config(userConfig, { command, isSsrBuild }) {
      const port = userConfig.server?.port ?? 5173
      return {
        // Built files live in public/build and are served at /build/.
        base: command === 'build' ? `/${buildDir}/` : '',
        publicDir: false,
        server: {
          port,
          strictPort: true,
          cors: true,
          // Assets (images, fonts) must load from the Vite server, not Gin.
          origin: userConfig.server?.origin ?? `http://localhost:${port}`,
        },
        build: {
          manifest: isSsrBuild ? false : true,
          ssrManifest: false,
          outDir: isSsrBuild ? ssrOutDir : path.join(publicDir, buildDir),
          emptyOutDir: true,
          rollupOptions: { input: isSsrBuild ? ssr : input },
        },
      }
    },

    configureServer(server) {
      server.httpServer?.once('listening', () => {
        const url = server.resolvedUrls?.local?.[0] ?? server.config.server.origin
        fs.mkdirSync(path.dirname(hot), { recursive: true })
        fs.writeFileSync(hot, url.replace(/\/$/, ''))
        setTimeout(() => {
          server.config.logger.info(`\n  ginertia  Go app will load assets from ${url}\n`)
        }, 50)
      })
      const clean = () => fs.existsSync(hot) && fs.rmSync(hot)
      process.on('exit', clean)
      process.on('SIGINT', () => process.exit())
      process.on('SIGTERM', () => process.exit())
      process.on('SIGHUP', () => process.exit())
    },
  }
}
