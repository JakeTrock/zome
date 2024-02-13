// Cynhyrchwyd y ffeil hon yn awtomatig. PEIDIWCH Â MODIWL
// This file is automatically generated. DO NOT EDIT
import {context} from '../models';

export function CloseDb():Promise<void>;

export function Count():Promise<void>;

export function DbRead(arg1:string,arg2:string):Promise<string>;

export function DbWrite(arg1:string,arg2:string,arg3:string):Promise<void>;

export function Decrypt(arg1:any):Promise<any>;

export function DumpBackup():Promise<void>;

export function Encrypt(arg1:string):Promise<any>;

export function GetStats():Promise<string>;

export function GetUUID():Promise<string>;

export function GetUUIDPretty():Promise<string>;

export function HandleEvents(arg1:context.Context):Promise<void>;

export function InitDb(arg1:{[key: string]: string}):Promise<void>;

export function InitP2P(arg1:context.Context):Promise<void>;

export function LoadConfig(arg1:{[key: string]: string}):Promise<void>;

export function RefreshPlugins():Promise<void>;

export function RestoreBackup(arg1:string):Promise<void>;