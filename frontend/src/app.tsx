import './App.css'
import logo from "./assets/images/logo-universal.png"
import {useRef, useState} from "preact/hooks";
import {h} from 'preact';
import {Events} from '@wailsio/runtime';

export function App(props: any) {
    const [resultText, setResultText] = useState<string[]>([]);
    const tbox = useRef<HTMLInputElement>(null);

    Events.On("peerData", (result: string) => {
        console.log("got peer data: ", result);
        setResultText([...resultText, "peer: "+result]);
    });

    const sendPeerData = async () => {
        const tbv = tbox.current?.value;
        if (!tbv) {
            return;
        }
        console.log("sending peer data: ", tbv);
        Events.Emit({name: "peerData", data: tbv});
        setResultText([...resultText, "me: " + tbv]);
        tbox.current!.value = '';
    }

    return (
        <>
            <div id="App">
                <div id="input" className="input-box">
                    <input id="name" className="input" ref={tbox} autoComplete="off" name="input"
                           type="text"/>
                    <button className="btn" onClick={sendPeerData}>send</button>
                </div>
                {resultText.map((result, index) => {
                    return <div className="result" key={index}>{result}</div>
                })}
            </div>
        </>
    )
}
