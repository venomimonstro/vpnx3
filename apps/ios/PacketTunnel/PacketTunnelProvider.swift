import Foundation
import NetworkExtension
import WireGuardKit

final class PacketTunnelProvider:NEPacketTunnelProvider {
    private lazy var adapter=WireGuardAdapter(with:self){_,_ in}

    override func startTunnel(
        options:[String:NSObject]?,
        completionHandler:@escaping(Error?)->Void
    ){
        do{
            let config=try tunnelConfiguration()
            adapter.start(tunnelConfiguration:config){[weak self] error in
                if error != nil{self?.closeWorkerSession()}
                completionHandler(error)
            }
        }catch{
            closeWorkerSession()
            completionHandler(error)
        }
    }

    override func stopTunnel(
        with reason:NEProviderStopReason,
        completionHandler:@escaping()->Void
    ){
        closeWorkerSession()
        adapter.stop{_ in completionHandler()}
    }

    private func tunnelConfiguration()throws->TunnelConfiguration{
        guard let proto=protocolConfiguration as? NETunnelProviderProtocol,
              let values=proto.providerConfiguration,
              let keyMode=values["keyMode"] as? String,
              let address=values["assignedIP"] as? String,
              let serverKeyText=values["serverPublicKey"] as? String,
              let serverKey=PublicKey(base64Key:serverKeyText),
              let endpointText=values["endpoint"] as? String,
              let endpoint=Endpoint(from:endpointText)
        else{throw PacketTunnelError.invalidConfiguration}

        let privateKey:PrivateKey
        switch keyMode{
        case "personal":privateKey=try PersonalKeyStore().ensure().privateKey
        case "standard":privateKey=try StandardTunnelKeyStore().ensure()
        default:throw PacketTunnelError.invalidConfiguration
        }

        guard let range=IPAddressRange(from:address) else{
            throw PacketTunnelError.invalidConfiguration
        }
        var iface=InterfaceConfiguration(privateKey:privateKey)
        iface.addresses=[range]
        iface.mtu=UInt16(min(max(values["mtu"] as? Int ?? 1280,576),1500))
        iface.dns=(values["dns"] as? [String] ?? []).compactMap{DNSServer(from:$0)}

        var peer=PeerConfiguration(publicKey:serverKey)
        guard let ipv4=IPAddressRange(from:"0.0.0.0/0"),
              let ipv6=IPAddressRange(from:"::/0") else{
            throw PacketTunnelError.invalidConfiguration
        }
        peer.allowedIPs=[ipv4,ipv6]
        peer.endpoint=endpoint
        peer.persistentKeepAlive=UInt16(min(max(values["keepalive"] as? Int ?? 25,0),120))
        return TunnelConfiguration(name:"VPNX3",interface:iface,peers:[peer])
    }

    private func closeWorkerSession(){
        guard let proto=protocolConfiguration as? NETunnelProviderProtocol,
              let values=proto.providerConfiguration,
              let sessionID=values["sessionID"] as? String,
              let sessionAPI=values["sessionAPI"] as? String else{return}
        let base=sessionAPI.hasSuffix("/") ? String(sessionAPI.dropLast()) : sessionAPI
        guard let url=URL(string:base+"/"+sessionID) else{return}
        var request=URLRequest(url:url)
        request.httpMethod="DELETE"
        request.timeoutInterval=5
        URLSession.shared.dataTask(with:request).resume()
    }
}

enum PacketTunnelError:Error{case invalidConfiguration}
