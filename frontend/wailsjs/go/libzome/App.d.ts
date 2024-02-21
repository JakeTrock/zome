// Cynhyrchwyd y ffeil hon yn awtomatig. PEIDIWCH Â MODIWL
// This file is automatically generated. DO NOT EDIT
import {bufio} from '../models';
import {os} from '../models';
import {multipart} from '../models';
import {context} from '../models';

export function DbClose():Promise<void>;

export function DbDelete(arg1:string,arg2:Array<string>):Promise<boolean>;

export function DbDumpBackup():Promise<boolean>;

export function DbInit(arg1:{[key: string]: string}):Promise<void>;

export function DbRead(arg1:string,arg2:Array<string>):Promise<Array<string>>;

export function DbRestoreBackup(arg1:string):Promise<boolean>;

export function DbStats():Promise<string>;

export function DbWrite(arg1:string,arg2:Array<string>,arg3:Array<string>):Promise<boolean>;

export function EcCheckSig(arg1:Array<number>,arg2:string,arg3:string):Promise<boolean>;

export function EcDecrypt(arg1:bufio.ReadWriter):Promise<void>;

export function EcEncrypt(arg1:string,arg2:number,arg3:bufio.ReadWriter):Promise<void>;

export function EcSign(arg1:Array<number>):Promise<string>;

export function FsCheckSigFile(arg1:string,arg2:string,arg3:string,arg4:string):Promise<boolean>;

export function FsCopyFileOrFolder(arg1:string,arg2:string,arg3:string,arg4:boolean):Promise<boolean>;

export function FsCreateFolder(arg1:string,arg2:string):Promise<boolean>;

export function FsCreateSandboxFolder(arg1:string):Promise<boolean>;

export function FsDeleteFileOrFolder(arg1:string,arg2:Array<string>,arg3:Array<boolean>):Promise<boolean>;

export function FsDownloadFile(arg1:string,arg2:string):Promise<os.File>;

export function FsGetDirectoryListing(arg1:string,arg2:string,arg3:boolean):Promise<string>;

export function FsGetHash(arg1:string,arg2:string,arg3:boolean):Promise<string>;

export function FsLoadConfig(arg1:{[key: string]: string}):Promise<void>;

export function FsMoveFileOrFolder(arg1:string,arg2:string,arg3:string):Promise<boolean>;

export function FsSaveConfig():Promise<void>;

export function FsSignFile(arg1:string,arg2:string):Promise<string>;

export function FsUploadFile(arg1:string,arg2:multipart.FileHeader):Promise<boolean>;

export function GetUUID():Promise<string>;

export function GetUUIDPretty():Promise<string>;

export function HandleEvents(arg1:context.Context):Promise<void>;

export function P2PGetPeers():Promise<Array<string>>;

export function P2PPushMessage(arg1:bufio.Reader,arg2:string,arg3:string):Promise<void>;
