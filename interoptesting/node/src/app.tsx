import { useState, useEffect } from 'preact/hooks'
import preactLogo from './assets/preact.svg'
import viteLogo from '/vite.svg'
import Libp2p from 'libp2p';
import TCP from 'libp2p-tcp';
const MulticastDNS = require('libp2p-mdns')
import KadDHT from 'libp2p-kad-dht';
import Gossipsub from 'libp2p-gossipsub';
import './app.css'

//https://github.com/libp2p/js-libp2p-example-browser-pubsub/blob/main/index.js

const topicName = 'crossPlatTopic';

export function App() {
  const [count, setCount] = useState(0)

  
  useEffect(()=>{
    async function main() {
      const libp2p = await Libp2p.createLibp2p({
        addresses: {
          listen: [
            // create listeners for incoming WebRTC connection attempts on on all
            // available Circuit Relay connections
            '/webrtc'
          ]
        },
        transports: [
          // the WebSocket transport lets us dial a local relay
          webSockets({
            // this allows non-secure WebSocket connections for purposes of the demo
            filter: filters.all
          }),
          // support dialing/listening on WebRTC addresses
          webRTC(),
          // support dialing/listening on Circuit Relay addresses
          circuitRelayTransport({
            // make a reservation on any discovered relays - this will let other
            // peers use the relay to contact us
            discoverRelays: 1
          })
        ],
        // a connection encrypter is necessary to dial the relay
        connectionEncryption: [noise()],
        // a stream muxer is necessary to dial the relay
        streamMuxers: [yamux()],
        connectionGater: {
          denyDialMultiaddr: () => {
            // by default we refuse to dial local addresses from browsers since they
            // are usually sent by remote peers broadcasting undialable multiaddrs and
            // cause errors to appear in the console but in this example we are
            // explicitly connecting to a local node so allow all addresses
            return false
          }
        },
        services: {
          identify: identify(),
          pubsub: newGossipsub(),
          dcutr: dcutr()
        },
        connectionManager: {
          minConnections: 0
        }
      });
    
      await libp2p.start();
      console.log('Libp2p started!');
    
      libp2p.pubsub.subscribe(topicName, (message) => {
        console.log(`${message.from}: ${message.data.toString()}`);
      });
    
      setInterval(async () => {
        console.log('SENDping');
        await libp2p.pubsub.publish(topicName, Buffer.from('ping'));
      }, 10000);
    }
    
    main().catch((err) => {
      console.error(err);
      process.exit(1);
    });
  })




  return (
    <>
      <div>
        <a href="https://vitejs.dev" target="_blank">
          <img src={viteLogo} class="logo" alt="Vite logo" />
        </a>
        <a href="https://preactjs.com" target="_blank">
          <img src={preactLogo} class="logo preact" alt="Preact logo" />
        </a>
      </div>
      <h1>Vite + Preact</h1>
      <div class="card">
        <button onClick={() => setCount((count) => count + 1)}>
          count is {count}
        </button>
        <p>
          Edit <code>src/app.tsx</code> and save to test HMR
        </p>
      </div>
      <p class="read-the-docs">
        Click on the Vite and Preact logos to learn more
      </p>
    </>
  )
}
