// Simple Express app to test LogNorth locally
// Usage: node examples/express-test.js <api_key>
// Example: node examples/express-test.js lb_abc123

const http = require('http')

const API_KEY = process.argv[2] || 'lb_test'
const LOGNORTH_URL = 'http://localhost:8080'

// Simple LogNorth client
function sendEvent(event) {
  const data = JSON.stringify(event)
  const url = new URL('/api/v1/events', LOGNORTH_URL)

  const req = http.request({
    hostname: url.hostname,
    port: url.port,
    path: url.pathname,
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${API_KEY}`,
      'Content-Length': data.length
    }
  })

  req.on('error', (e) => console.error('LogNorth error:', e.message))
  req.write(data)
  req.end()
}

// Middleware to log requests to LogNorth
function logRequest(req, res, next) {
  const start = Date.now()
  const traceId = `req-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`

  res.on('finish', () => {
    const duration = Date.now() - start
    const isError = res.statusCode >= 500

    sendEvent({
      message: `${req.method} ${req.url} → ${res.statusCode}`,
      duration_ms: duration,
      trace_id: traceId,
      context: {
        method: req.method,
        path: req.url,
        status: res.statusCode,
        ip: req.socket.remoteAddress,
        ...(isError && { error: res.errorMessage || 'Internal Server Error' })
      }
    })
  })

  req.traceId = traceId
  next()
}

// Simple router
const routes = {
  'GET /': (req, res) => {
    res.json({ message: 'Hello from Express test app!' })
  },

  'GET /users': (req, res) => {
    res.json([
      { id: 1, name: 'Alice' },
      { id: 2, name: 'Bob' }
    ])
  },

  'GET /users/:id': (req, res) => {
    const id = req.params.id
    if (id === '999') {
      res.statusCode = 404
      res.errorMessage = 'User not found'
      return res.json({ error: 'User not found' })
    }
    res.json({ id, name: 'User ' + id })
  },

  'POST /checkout': (req, res) => {
    // Simulate random failures
    if (Math.random() < 0.3) {
      res.statusCode = 500
      res.errorMessage = 'Payment gateway error: card declined'
      return res.json({ error: 'Payment failed' })
    }
    res.json({ success: true, orderId: Date.now() })
  },

  'GET /slow': async (req, res) => {
    await new Promise(r => setTimeout(r, 2000))
    res.json({ message: 'Finally done!' })
  },

  'GET /error': (req, res) => {
    res.statusCode = 500
    res.errorMessage = 'Database connection timeout'
    res.json({ error: 'Something went wrong' })
  },

  'GET /health': (req, res) => {
    res.json({ status: 'ok' })
  }
}

// Mini Express-like framework
function createApp() {
  const middlewares = []

  const app = (req, res) => {
    // Parse URL
    const url = new URL(req.url, `http://${req.headers.host}`)
    req.path = url.pathname
    req.query = Object.fromEntries(url.searchParams)

    // Simple param extraction
    req.params = {}

    // JSON response helper
    res.json = (data) => {
      res.setHeader('Content-Type', 'application/json')
      res.end(JSON.stringify(data))
    }

    // Run middlewares then route
    let i = 0
    const next = () => {
      if (i < middlewares.length) {
        middlewares[i++](req, res, next)
      } else {
        // Find matching route
        for (const [pattern, handler] of Object.entries(routes)) {
          const [method, path] = pattern.split(' ')
          if (req.method !== method) continue

          // Check for param routes like /users/:id
          const paramMatch = path.match(/^(.+)\/:(\w+)$/)
          if (paramMatch) {
            const [, base, param] = paramMatch
            const regex = new RegExp(`^${base}/([^/]+)$`)
            const match = req.path.match(regex)
            if (match) {
              req.params[param] = match[1]
              return handler(req, res)
            }
          } else if (req.path === path) {
            return handler(req, res)
          }
        }

        // 404
        res.statusCode = 404
        res.json({ error: 'Not found' })
      }
    }
    next()
  }

  app.use = (fn) => middlewares.push(fn)
  return app
}

// Start server
const app = createApp()
app.use(logRequest)

const server = http.createServer(app)
const PORT = 3000

server.listen(PORT, () => {
  console.log(`
Express test app running on http://localhost:${PORT}
Sending events to ${LOGNORTH_URL} with key ${API_KEY.slice(0, 10)}...

Try these endpoints:
  curl http://localhost:${PORT}/
  curl http://localhost:${PORT}/users
  curl http://localhost:${PORT}/users/123
  curl http://localhost:${PORT}/users/999     # 404
  curl -X POST http://localhost:${PORT}/checkout  # random failures
  curl http://localhost:${PORT}/slow          # 2s delay
  curl http://localhost:${PORT}/error         # 500 error
`)
})
