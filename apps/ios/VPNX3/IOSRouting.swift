import Foundation

struct IOSNetworkEndpoint {
    let kind:String
    let transport:String
    let scheme:String
    let host:String
    let port:Int
    let path:String
    let priority:Int

    var hostPort:String { "\(host):\(port)" }

    var httpsURL:URL? {
        guard scheme=="https" else{return nil}
        return URL(string:"https://\(host):\(port)\(path)")
    }
}

struct IOSWorkerRoute {
    let nodeID:String
    let sessionAPI:IOSNetworkEndpoint
    let wireGuard:IOSNetworkEndpoint
    let priority:Int
    let latency:Double
    let health:Double
}

enum IOSRouting {
    static func candidates(_ config:IOSVerifiedConfig)->[IOSWorkerRoute]{
        guard let workers=config.json["workers"] as? [[String:Any]] else{return []}
        var routes:[IOSWorkerRoute]=[]

        for worker in workers {
            guard let nodeID=worker["id"] as? String,
                  let rawEndpoints=worker["endpoints"] as? [[String:Any]] else{continue}
            let endpoints=rawEndpoints.compactMap(parse)
            let sessions=endpoints.filter{$0.kind=="session_api" && $0.transport=="wireguard" && $0.scheme=="https"}
            let wireguards=endpoints.filter{$0.kind=="wireguard" && $0.transport=="wireguard" && $0.scheme=="udp"}
            guard let session=sessions.min(by:{$0.priority<$1.priority}),
                  let wg=wireguards.min(by:{$0.priority<$1.priority}) else{continue}

            routes.append(IOSWorkerRoute(
                nodeID:nodeID,
                sessionAPI:session,
                wireGuard:wg,
                priority:session.priority+wg.priority,
                latency:(worker["latency_ms"] as? NSNumber)?.doubleValue ?? Double.greatestFiniteMagnitude,
                health:(worker["health_score"] as? NSNumber)?.doubleValue ?? 0
            ))
        }

        return routes.sorted{
            if $0.priority != $1.priority { return $0.priority < $1.priority }
            if $0.latency != $1.latency { return $0.latency < $1.latency }
            return $0.health > $1.health
        }
    }

    private static func parse(_ raw:[String:Any])->IOSNetworkEndpoint?{
        guard let kind=raw["kind"] as? String,
              let transport=raw["transport"] as? String,
              let scheme=raw["scheme"] as? String,
              let host=raw["host"] as? String,
              let port=(raw["port"] as? NSNumber)?.intValue,
              (1...65535).contains(port)
        else{return nil}
        return IOSNetworkEndpoint(
            kind:kind,transport:transport,scheme:scheme,host:host,port:port,
            path:raw["path"] as? String ?? "",
            priority:(raw["priority"] as? NSNumber)?.intValue ?? 100
        )
    }
}
