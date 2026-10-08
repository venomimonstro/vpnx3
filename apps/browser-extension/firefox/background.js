importScripts("runtime-config.js");

const api = typeof browser !== "undefined" ? browser : chrome;
const isFirefox = typeof browser !== "undefined";

const STATE_KEYS = {
  enabled: "enabled",
  deviceId: "device_id",
  userId: "user_id",
  sequence: "request_sequence",
  credential: "proxy_credential",
  credentialExpires: "proxy_credential_expires",
  ingress: "ingress",
  configVersion: "config_version"
};

function b64url(bytes) {
  let s="";
  const u=new Uint8Array(bytes);
  for(let i=0;i<u.length;i++) s+=String.fromCharCode(u[i]);
  return btoa(s).replace(/\+/g,"-").replace(/\//g,"_").replace(/=+$/,"");
}
function unb64url(s) {
  s=s.replace(/-/g,"+").replace(/_/g,"/");
  while(s.length%4) s+="=";
  const raw=atob(s);const out=new Uint8Array(raw.length);
  for(let i=0;i<raw.length;i++) out[i]=raw.charCodeAt(i);
  return out;
}
function hex(bytes){return [...new Uint8Array(bytes)].map(x=>x.toString(16).padStart(2,"0")).join("")}
async function sha256(bytes){return crypto.subtle.digest("SHA-256",bytes)}

function rawEcdsaToDer(raw) {
  const sig=new Uint8Array(raw);
  if(sig.length!==64) return sig;
  const encInt=(part)=>{
    let i=0;while(i<part.length-1&&part[i]===0)i++;
    let v=part.slice(i);
    if(v[0]&0x80){const x=new Uint8Array(v.length+1);x.set(v,1);v=x}
    const out=new Uint8Array(2+v.length);out[0]=0x02;out[1]=v.length;out.set(v,2);return out;
  };
  const r=encInt(sig.slice(0,32)),s=encInt(sig.slice(32));
  const seq=new Uint8Array(2+r.length+s.length);seq[0]=0x30;seq[1]=r.length+s.length;seq.set(r,2);seq.set(s,2+r.length);
  return seq;
}

function openIdentityDB() {
  return new Promise((resolve,reject)=>{
    const req=indexedDB.open("vpnx3_identity",1);
    req.onupgradeneeded=()=>req.result.createObjectStore("keys");
    req.onsuccess=()=>resolve(req.result);req.onerror=()=>reject(req.error);
  });
}
async function dbGet(key){
  const db=await openIdentityDB();
  return new Promise((resolve,reject)=>{
    const tx=db.transaction("keys","readonly");const r=tx.objectStore("keys").get(key);
    r.onsuccess=()=>resolve(r.result);r.onerror=()=>reject(r.error);
  });
}
async function dbPut(key,value){
  const db=await openIdentityDB();
  return new Promise((resolve,reject)=>{
    const tx=db.transaction("keys","readwrite");tx.objectStore("keys").put(value,key);
    tx.oncomplete=()=>resolve();tx.onerror=()=>reject(tx.error);
  });
}
async function identity(){
  let privateKey=await dbGet("private");
  let publicSpki=await dbGet("public_spki");
  if(privateKey&&publicSpki) return {privateKey,publicSpki};

  const pair=await crypto.subtle.generateKey(
    {name:"ECDSA",namedCurve:"P-256"},true,["sign","verify"]
  );
  const pkcs8=await crypto.subtle.exportKey("pkcs8",pair.privateKey);
  const spki=await crypto.subtle.exportKey("spki",pair.publicKey);
  privateKey=await crypto.subtle.importKey(
    "pkcs8",pkcs8,{name:"ECDSA",namedCurve:"P-256"},false,["sign"]
  );
  publicSpki=spki;
  await dbPut("private",privateKey);await dbPut("public_spki",publicSpki);
  return {privateKey,publicSpki};
}

async function signedHeaders(method,path,body,deviceId) {
  const id=await identity();
  const ts=Math.floor(Date.now()/1000).toString();
  const digest=await sha256(body);
  const canonical=new TextEncoder().encode([method,path,ts,hex(digest)].join("\n"));
  const raw=await crypto.subtle.sign({name:"ECDSA",hash:"SHA-256"},id.privateKey,canonical);
  const der=rawEcdsaToDer(raw);
  const headers={
    "Content-Type":"application/json",
    "Accept":"application/json",
    "X-VPNX3-Timestamp":ts,
    "X-VPNX3-Signature":b64url(der)
  };
  if(deviceId) headers["X-VPNX3-Device-ID"]=deviceId;
  return headers;
}

async function request(method,path,bodyObj,deviceId) {
  const body=bodyObj==null?null:new TextEncoder().encode(JSON.stringify(bodyObj));
  const headers=body?await signedHeaders(method,path,body,deviceId):{"Accept":"application/json"};
  const res=await fetch(VPNX3_CONTROL_URL.replace(/\/$/,"")+path,{
    method,headers,body:body?body:undefined,cache:"no-store"
  });
  const text=await res.text();
  if(!res.ok) throw new Error("HTTP "+res.status+" "+text.slice(0,200));
  return text?JSON.parse(text):null;
}

async function storageGet(keys){return api.storage.local.get(keys)}
async function storageSet(values){return api.storage.local.set(values)}

async function ensureRegistered(){
  const s=await storageGet([STATE_KEYS.deviceId,STATE_KEYS.userId]);
  if(s[STATE_KEYS.deviceId]&&s[STATE_KEYS.userId]) return s;
  const id=await identity();
  const platform=isFirefox?"firefox":"chrome";
  const path="/api/v1/client/register";
  const body={
    platform,
    display_name:(isFirefox?"Firefox":"Chrome")+" extension",
    identity_algorithm:"ecdsa-p256-sha256",
    public_key:b64url(id.publicSpki)
  };
  const reg=await request("POST",path,body,null);
  await storageSet({
    [STATE_KEYS.deviceId]:reg.device_id,
    [STATE_KEYS.userId]:reg.user_id,
    [STATE_KEYS.sequence]:0
  });
  return {[STATE_KEYS.deviceId]:reg.device_id,[STATE_KEYS.userId]:reg.user_id};
}

async function nextSequence(){
  const s=await storageGet([STATE_KEYS.sequence]);
  const next=Number(s[STATE_KEYS.sequence]||0)+1;
  await storageSet({[STATE_KEYS.sequence]:next});
  return next;
}

async function verifyConfig(envelope){
  const pub=unb64url(VPNX3_CONFIG_PUBLIC_KEY);
  if(pub.length!==32) throw new Error("Pinned config public key is invalid");
  const payload=unb64url(envelope.payload);
  const signature=unb64url(envelope.signature);
  const key=await crypto.subtle.importKey("raw",pub,{name:"Ed25519"},false,["verify"]);
  if(!await crypto.subtle.verify({name:"Ed25519"},key,signature,payload)) throw new Error("Invalid configuration signature");
  const digest=await sha256(pub);
  if(hex(digest).slice(0,16)!==envelope.key_id) throw new Error("Unexpected configuration signing key");
  const cfg=JSON.parse(new TextDecoder().decode(payload));
  if(cfg.schema_version!==1) throw new Error("Unsupported configuration schema");
  const now=Date.now();
  if(Date.parse(cfg.created_at)>now+5*60*1000) throw new Error("Configuration is from the future");
  if(Date.parse(cfg.expires_at)<=now) throw new Error("Configuration expired");
  const s=await storageGet([STATE_KEYS.configVersion]);
  const highest=Number(s[STATE_KEYS.configVersion]||0);
  if(Number(cfg.version)<highest) throw new Error("Configuration rollback detected");
  await storageSet({[STATE_KEYS.configVersion]:Math.max(highest,Number(cfg.version))});
  return cfg;
}

async function latestIngress(){
  const env=await request("GET","/api/v1/config/latest",null,null);
  const cfg=await verifyConfig(env);
  const candidates=[];
  for(const node of (cfg.ingresses||[])){
    for(const ep of (node.endpoints||[])){
      if(ep.kind==="ingress"&&ep.scheme==="https"&&ep.transport==="http-connect"){
        candidates.push({
          host:ep.host,port:ep.port,priority:ep.priority||100,
          latency:Number.isFinite(node.latency_ms)?node.latency_ms:Number.MAX_SAFE_INTEGER,
          health:node.health_score??0,nodeId:node.id
        });
      }
    }
  }
  candidates.sort((a,b)=>a.priority-b.priority||a.latency-b.latency||b.health-a.health);
  if(!candidates.length) throw new Error("No active browser ingress");
  return candidates[0];
}

async function accountStatus(){
  const reg=await ensureRegistered();
  const sequence=await nextSequence();
  return request("POST","/api/v1/client/account/status",{sequence},reg[STATE_KEYS.deviceId]);
}

async function createPairingCode(){
  const reg=await ensureRegistered();
  const sequence=await nextSequence();
  return request("POST","/api/v1/client/pairing-code",{sequence},reg[STATE_KEYS.deviceId]);
}

async function claimPairingCode(code){
  const reg=await ensureRegistered();
  const sequence=await nextSequence();
  const status=await request("POST","/api/v1/client/pairing-claim",{
    sequence,
    code:String(code||"").trim().toUpperCase()
  },reg[STATE_KEYS.deviceId]);
  await storageSet({[STATE_KEYS.userId]:status.user_id});
  return status;
}

async function referralCode(){
  const reg=await ensureRegistered();
  const sequence=await nextSequence();
  return request("POST","/api/v1/client/referral/code",{sequence},reg[STATE_KEYS.deviceId]);
}

async function referralStatus(){
  const reg=await ensureRegistered();
  const sequence=await nextSequence();
  return request("POST","/api/v1/client/referral/status",{sequence},reg[STATE_KEYS.deviceId]);
}

async function referralClaim(code){
  const reg=await ensureRegistered();
  const sequence=await nextSequence();
  return request("POST","/api/v1/client/referral/claim",{
    sequence,code:String(code||"").trim().toUpperCase()
  },reg[STATE_KEYS.deviceId]);
}

async function newProxyCredential(){
  const reg=await ensureRegistered();
  const sequence=await nextSequence();
  const lease=await request("POST","/api/v1/client/proxy-lease",{sequence},reg[STATE_KEYS.deviceId]);
  await storageSet({
    [STATE_KEYS.credential]:lease.credential,
    [STATE_KEYS.credentialExpires]:lease.expires_at
  });
  return lease;
}

async function ensureCredential(){
  const s=await storageGet([STATE_KEYS.credential,STATE_KEYS.credentialExpires]);
  if(s[STATE_KEYS.credential]&&Date.parse(s[STATE_KEYS.credentialExpires]||0)>Date.now()+5*60*1000){
    return s[STATE_KEYS.credential];
  }
  return (await newProxyCredential()).credential;
}

async function applyProxy(ingress){
  if(isFirefox){
    if(api.proxy.onRequest){
      // onRequest listener is installed once below; it reads current ingress from storage.
      await storageSet({[STATE_KEYS.ingress]:ingress});
    }else{
      await api.proxy.settings.set({value:{
        proxyType:"manual",
        http:ingress.host+":"+ingress.port,
        ssl:ingress.host+":"+ingress.port,
        proxyDNS:true
      }});
    }
  }else{
    await api.proxy.settings.set({
      value:{mode:"fixed_servers",rules:{
        singleProxy:{scheme:"https",host:ingress.host,port:ingress.port},
        bypassList:["<local>","localhost","127.0.0.1"]
      }},
      scope:"regular"
    });
  }
}

async function clearProxy(){
  if(isFirefox){
    await storageSet({[STATE_KEYS.ingress]:null});
    if(api.proxy.settings) await api.proxy.settings.clear({});
  }else await api.proxy.settings.clear({scope:"regular"});
}

const MAINTENANCE_ALARM="vpnx3-maintenance";

function sameIngress(a,b){
  return !!a && !!b && a.host===b.host && Number(a.port)===Number(b.port);
}

async function scheduleMaintenance(){
  if(api.alarms){
    await api.alarms.create(MAINTENANCE_ALARM,{periodInMinutes:1});
  }
}

async function maintainConnection(){
  const s=await storageGet([STATE_KEYS.enabled,STATE_KEYS.ingress]);
  if(!s[STATE_KEYS.enabled]) return null;

  // Refresh authentication before it reaches its expiry and then read the
  // latest signed routing manifest. Failure here does not clear the current
  // proxy: a temporary Control Plane outage must not cause an avoidable leak.
  await ensureCredential();
  const ingress=await latestIngress();
  if(!sameIngress(s[STATE_KEYS.ingress],ingress)){
    await storageSet({[STATE_KEYS.ingress]:ingress});
    await applyProxy(ingress);
  }
  return ingress;
}

async function connect(){
  await ensureRegistered();
  await ensureCredential();
  const ingress=await latestIngress();
  await storageSet({[STATE_KEYS.enabled]:true,[STATE_KEYS.ingress]:ingress});
  await applyProxy(ingress);
  await scheduleMaintenance();
  return ingress;
}
async function disconnect(){
  await storageSet({[STATE_KEYS.enabled]:false,[STATE_KEYS.ingress]:null});
  if(api.alarms) await api.alarms.clear(MAINTENANCE_ALARM);
  await clearProxy();
}

if(isFirefox&&api.proxy.onRequest){
  api.proxy.onRequest.addListener(async()=>{
    const s=await storageGet([STATE_KEYS.enabled,STATE_KEYS.ingress]);
    if(!s[STATE_KEYS.enabled]||!s[STATE_KEYS.ingress]) return {type:"direct"};
    const i=s[STATE_KEYS.ingress];
    return {type:"https",host:i.host,port:i.port,proxyDNS:true,failoverTimeout:5};
  },{urls:["<all_urls>"]});
}

const authAttempts=new Map();

api.webRequest.onAuthRequired.addListener(
  (details,callback)=>{
    const attempts=(authAttempts.get(details.requestId)||0)+1;
    authAttempts.set(details.requestId,attempts);

    let credentialPromise;
    if(attempts===1) credentialPromise=ensureCredential();
    else if(attempts===2) credentialPromise=newProxyCredential();
    else {
      authAttempts.delete(details.requestId);
      callback({cancel:true});
      return;
    }

    credentialPromise
      .then(credential=>callback({authCredentials:{username:"vpnx3",password:credential}}))
      .catch(()=>{
        authAttempts.delete(details.requestId);
        callback({cancel:true});
      });
  },
  {urls:["<all_urls>"]},
  ["asyncBlocking"]
);

api.webRequest.onCompleted.addListener(
  details=>authAttempts.delete(details.requestId),
  {urls:["<all_urls>"]}
);
api.webRequest.onErrorOccurred.addListener(
  details=>authAttempts.delete(details.requestId),
  {urls:["<all_urls>"]}
);

api.runtime.onMessage.addListener((message,sender,sendResponse)=>{
  if(message?.type==="connect"){
    connect().then(i=>sendResponse({ok:true,ingress:i})).catch(e=>sendResponse({ok:false,error:e.message}));
    return true;
  }
  if(message?.type==="disconnect"){
    disconnect().then(()=>sendResponse({ok:true})).catch(e=>sendResponse({ok:false,error:e.message}));
    return true;
  }
  if(message?.type==="status"){
    Promise.all([
      storageGet([STATE_KEYS.enabled,STATE_KEYS.ingress,STATE_KEYS.credentialExpires]),
      accountStatus().catch(()=>null)
    ]).then(([s,account])=>sendResponse({
      ok:true,enabled:!!s[STATE_KEYS.enabled],ingress:s[STATE_KEYS.ingress]||null,
      expires:s[STATE_KEYS.credentialExpires]||null,account
    }));
    return true;
  }
  if(message?.type==="pairing-create"){
    createPairingCode()
      .then(x=>sendResponse({ok:true,code:x.code,expires:x.expires_at}))
      .catch(e=>sendResponse({ok:false,error:e.message}));
    return true;
  }
  if(message?.type==="pairing-claim"){
    claimPairingCode(message.code||"")
      .then(account=>sendResponse({ok:true,account}))
      .catch(e=>sendResponse({ok:false,error:e.message}));
    return true;
  }
  if(message?.type==="referral-status"){
    Promise.all([referralCode(),referralStatus()])
      .then(([code,status])=>sendResponse({ok:true,code:code.code,status}))
      .catch(e=>sendResponse({ok:false,error:e.message}));
    return true;
  }
  if(message?.type==="referral-claim"){
    referralClaim(message.code||"")
      .then(status=>sendResponse({ok:true,status}))
      .catch(e=>sendResponse({ok:false,error:e.message}));
    return true;
  }
});

if(api.alarms){
  api.alarms.onAlarm.addListener(alarm=>{
    if(alarm.name===MAINTENANCE_ALARM){
      maintainConnection().catch(()=>{});
    }
  });
}

if(api.proxy?.onProxyError){
  api.proxy.onProxyError.addListener(()=>maintainConnection().catch(()=>{}));
}
if(api.proxy?.onError){
  api.proxy.onError.addListener(()=>maintainConnection().catch(()=>{}));
}

api.runtime.onStartup.addListener(async()=>{
  const s=await storageGet([STATE_KEYS.enabled]);
  if(s[STATE_KEYS.enabled]){
    await scheduleMaintenance();
    maintainConnection().catch(()=>{});
  }
});
api.runtime.onInstalled.addListener(async()=>{
  const s=await storageGet([STATE_KEYS.enabled]);
  if(s[STATE_KEYS.enabled]) await scheduleMaintenance();
});
