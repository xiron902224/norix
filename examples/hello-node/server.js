const http = require('http')
const port = process.env.PORT || 8080
http
  .createServer((_req, res) => {
    res.writeHead(200, { 'Content-Type': 'text/plain' })
    res.end('hello from norix railpack node\n')
  })
  .listen(port, () => console.log(`listening on :${port}`))
