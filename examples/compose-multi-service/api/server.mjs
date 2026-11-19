import http from 'node:http'
import net from 'node:net'

const port = Number(process.env.PORT || 8080)
const dbHost = process.env.DB_HOST || 'db'
const dbPort = Number(process.env.DB_PORT || 5432)

// Opens a TCP connection to the database service name. On a Norix project
// network this resolves through the service alias, so it proves #144 works.
function probe(host, targetPort) {
  return new Promise((resolve) => {
    const socket = net.connect({ host, port: targetPort, timeout: 2000 })
    socket.once('connect', () => {
      socket.destroy()
      resolve('reachable')
    })
    socket.once('timeout', () => {
      socket.destroy()
      resolve('timeout')
    })
    socket.once('error', () => resolve('unreachable'))
  })
}

http
  .createServer(async (req, res) => {
    if (req.url === '/healthz') {
      res.writeHead(200, { 'Content-Type': 'text/plain' })
      res.end('ok\n')
      return
    }
    if (req.url !== '/') {
      res.writeHead(404)
      res.end()
      return
    }
    const db = await probe(dbHost, dbPort)
    res.writeHead(200, { 'Content-Type': 'application/json' })
    res.end(JSON.stringify({ service: 'api', db }) + '\n')
  })
  .listen(port, () => console.log(`api listening on :${port}`))
