import { connect } from 'node:net'

const fallbackPort = process.env.WAILS_VITE_PORT || '9245'
const frontendURL = new URL(
  process.env.FRONTEND_DEVSERVER_URL || `http://127.0.0.1:${fallbackPort}`,
)
const port = Number(frontendURL.port || (frontendURL.protocol === 'https:' ? 443 : 80))
const deadline = Date.now() + 60_000

while (Date.now() < deadline) {
  const ready = await new Promise((resolve) => {
    const socket = connect({ host: frontendURL.hostname, port })
    let settled = false

    const finish = (result) => {
      if (settled) {
        return
      }
      settled = true
      socket.destroy()
      resolve(result)
    }

    socket.setTimeout(1_000)
    socket.once('connect', () => finish(true))
    socket.once('error', () => finish(false))
    socket.once('timeout', () => finish(false))
  })

  if (ready) {
    process.exit(0)
  }

  await new Promise((resolve) => setTimeout(resolve, 250))
}

console.error(
  `Frontend development server did not become ready at ${frontendURL.origin} within 60 seconds`,
)
process.exit(1)
