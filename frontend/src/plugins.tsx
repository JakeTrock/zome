import { RequestType, ZomeAccess } from "./access"
import { ZomeEvent, openSSE } from "./network"

interface PluginDescriptor {
    hashId: string,
    name: string,
    description: string,
    permissions: RequestType[],
    version: string,
}

export class PluginManager {
    private plugins: ZomePlugin[]
    constructor(pluginsFlat: PluginDescriptor[]) {
        this.plugins = pluginsFlat.map(plugin => 
            new ZomePlugin(new ZomeAccess(plugin.hashId, plugin.permissions)))
    }
    public drawInterface = () => {//TODO: switch out for a tab carousel, bottom tab is settings
        return (
            <div>
                <h1>Plugin Manager</h1>
                {this.plugins.map(p => {
                    return (<>
                        <div style={{border:"2px solid black"}}>
                        {p.drawInterface()}
                        </div>
                    </>)
                })}
            </div>
        )
    }
}

export class ZomePlugin {
    private apiAccess: ZomeAccess
    constructor(apiAccess: ZomeAccess) {
        this.apiAccess = apiAccess
    }

    public drawInterface = () => {
        return (
            <div>
                <h1>Interface</h1>
            </div>
        )
    }
}

// class ExamplePlugin extends ZomePlugin {
//     public drawInterface = () => {
//         return (
//             <div>
//                 <h1>Example Interface</h1>
//             </div>
//         )
//     }
// }