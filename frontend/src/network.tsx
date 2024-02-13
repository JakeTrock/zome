import { FunctionType, RequestType } from "./access";

const controlUrl = "http://localhost:8000/"//TODO: this should be a global state thing so you can set it

export interface ZomeEvent {
    applicationTargetId: string,
    requestId: string,
    requestType: RequestType,
    functionType: FunctionType[RequestType],
    data: string
}

type SSEType = "p2p-rec" | "fs-filechange"
export const openSSE = (type: SSEType, appRequesting:string, writeFunc: (event:ZomeEvent)=>void) => {
            const source = new EventSource(`${controlUrl}/${type}?rqBy=${appRequesting}`);

            source.onopen = function(event) {
                console.log("connected", event);
            }

            source.onerror = function(event) {
                console.log("error", event); //TODO: handle this better
            }

            source.onmessage = function(event) {
                console.log("message", event);

                try{
                    const eventJson = JSON.parse(event.data) as ZomeEvent
                    return writeFunc(eventJson)
                } catch (e) {
                    console.log("error parsing packet", e)
                }
            };
}

export const callEndpoint = (applicationTargetId: string, requestType: RequestType, functionType: string ) => {//TODO: switch back to FunctionType[RequestType]
    const requestId = Math.random().toString(36).substring(7)//TODO: better ID scheme?
    return (data: string | number | boolean | JSON)=>fetch(controlUrl, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json'
        },
        body: JSON.stringify({
            requestId,
            requestType,
            functionType,
            data,
            applicationTargetId,//TODO: implement this, probably set it in the constructor, it will be issued by master init
        })
    }).then(response => {
        if (response.ok) {
            return {
                requestId,
                data: response.json()
            }
        }
        return Promise.reject(response)
    })
}
