import { ZomeEvent, callEndpoint, openSSE } from "./network"

export class Access{
    public applicationTargetId: string
    constructor(applicationTargetId: string) {
        this.applicationTargetId = applicationTargetId
    }
}

export class DatabaseAccess extends Access{
    
    constructor(applicationTargetId: string) {
        super(applicationTargetId)
    }
    public create = (data: string) => 
        callEndpoint(this.applicationTargetId, "database", "create")(data)
    public read = (data: string) =>
        callEndpoint(this.applicationTargetId, "database", "read")(data)
    public update = (data: string) =>
        callEndpoint(this.applicationTargetId, "database", "update")(data)
    public delete = (data: string) =>
        callEndpoint(this.applicationTargetId, "database", "delete")(data)
}

export class P2PAccess extends Access{
    constructor(applicationTargetId: string) {
        super(applicationTargetId)
    }
    public send = (data: string) =>
        callEndpoint(this.applicationTargetId, "p2p", "send")(data)
    public receive = (writeFunc: (event:ZomeEvent)=>void) =>
        openSSE("p2p-rec", this.applicationTargetId, writeFunc)
}

export class EncryptionAccess extends Access{
    constructor(applicationTargetId: string) {
        super(applicationTargetId)
    }
    public encrypt = (data: string) =>
        callEndpoint(this.applicationTargetId, "encryption", "encrypt")(data)
    public decrypt = (data: string) =>
        callEndpoint(this.applicationTargetId, "encryption", "decrypt")(data)
}

export class FileSystemAccess extends Access{
    constructor(applicationTargetId: string) {
        super(applicationTargetId)
    }
    public read = (data: string) =>
        callEndpoint(this.applicationTargetId, "fs", "read")(data)
    public write = (data: string) =>
        callEndpoint(this.applicationTargetId, "fs", "write")(data)
    public watch = (writeFunc: (event:ZomeEvent)=>void) =>
        openSSE("fs-filechange", this.applicationTargetId, writeFunc)
}

export type RequestType = "database" | "p2p" | "encryption" | "fs"
export type FunctionType = {
    [K in RequestType]: K extends "database" ? keyof typeof DatabaseAccess
                      : K extends "p2p" ? keyof typeof P2PAccess
                      : K extends "encryption" ? keyof typeof EncryptionAccess
                      : K extends "fs" ? keyof typeof FileSystemAccess
                      : never;
}

export class ZomeAccess {
    public methods: Record<string, Access> = {}
    constructor(applicationTargetId: string, permissions: RequestType[]) {
        if(permissions.includes("database")) {
            this.methods = {
                ...this.methods, 
                database: new DatabaseAccess(applicationTargetId)
            }
        }
        if(permissions.includes("p2p")) {
            this.methods = {
                ...this.methods, 
                p2p: new P2PAccess(applicationTargetId)
            }
        }
        if(permissions.includes("encryption")) {
            this.methods = {
                ...this.methods, 
                encryption: new EncryptionAccess(applicationTargetId)
            }
        }
        if(permissions.includes("fs")) {
            this.methods = {
                ...this.methods, 
                fs: new FileSystemAccess(applicationTargetId)
            }
        }
    }
    

}