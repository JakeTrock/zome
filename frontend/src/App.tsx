import './App.css'
import {EventsOn, EventsEmit} from '../wailsjs/runtime/runtime';
import React from 'react';

interface Message {
    Message: string,
    SenderID: string,
    SenderNick: string,
}

function App() {
    
    const [messages, setMessages] = React.useState<Message[]>([])
    const [peers, setPeers] = React.useState<string[]>([])
    const tbox = React.useRef<HTMLInputElement>(null)


    React.useEffect(() => {
        EventsOn("system-message", m => {
            setMessages(om=>[...om, m as Message])
        })
        EventsOn("system-peers", p => setPeers(p))
    }, [])

    return (
        <div className="min-h-screen bg-white grid grid-cols-1 place-items-center justify-items-center mx-auto py-8">
            <div className="text-blue-900 text-2xl font-bold font-mono">
                <input type="text" className='bg-blue-900 text-white' ref={tbox}/>
                <button onClick={() => {
                    if (tbox.current) {
                        EventsEmit("frontend-message", tbox.current.value)
                        setMessages([...messages, {Message: tbox.current.value, SenderID: "", SenderNick: "Me"}])
                        tbox.current.value = ""
                    }
                }}>Send</button>
            </div>
            <div className="w-fit max-w-md">
                <h2>Peers</h2>
                {peers.map((p, i) => <p key={i}>{p}</p>)}
                <h2>Messages</h2>
                {messages.map((m, i) => <p className={m.SenderID && peers.includes(m.SenderID) ? "text-red-400" : ""} key={i}>{m.SenderNick}: {m.Message}</p>)}
            </div>
        </div>
    )
}

export default App
