package ru.vpnx3.app.data

import org.json.JSONObject
import ru.vpnx3.app.security.VerifiedConfig

data class NetworkEndpoint(
    val scheme: String,
    val host: String,
    val port: Int,
    val path: String,
    val priority: Int
) {
    fun httpsUrl(): String {
        require(scheme == "https")
        val normalizedPath = if (path.isBlank()) "" else path
        return "https://$host:$port$normalizedPath"
    }

    fun hostPort(): String = "$host:$port"
}

data class WorkerRoute(
    val nodeId: String,
    val sessionApi: NetworkEndpoint,
    val wireGuard: NetworkEndpoint
)

object RoutingSelector {
    fun select(config: VerifiedConfig): WorkerRoute {
        val workers = config.payload.getJSONArray("workers")
        val candidates = buildList {
            for (i in 0 until workers.length()) {
                val worker = workers.getJSONObject(i)
                val endpoints = worker.getJSONArray("endpoints")
                var sessionApi: NetworkEndpoint? = null
                var wireGuard: NetworkEndpoint? = null

                for (j in 0 until endpoints.length()) {
                    val endpoint = endpoints.getJSONObject(j)
                    val parsed = parse(endpoint)
                    when (endpoint.getString("kind")) {
                        "session_api" -> if (
                            endpoint.getString("transport") == "wireguard" &&
                            (sessionApi == null || parsed.priority < sessionApi!!.priority)
                        ) sessionApi = parsed
                        "wireguard" -> if (
                            endpoint.getString("transport") == "wireguard" &&
                            (wireGuard == null || parsed.priority < wireGuard!!.priority)
                        ) wireGuard = parsed
                    }
                }

                if (sessionApi != null && wireGuard != null) {
                    val health = if (worker.has("health_score") && !worker.isNull("health_score")) {
                        worker.getDouble("health_score")
                    } else {
                        0.0
                    }
                    add(
                        Triple(
                            WorkerRoute(
                                nodeId = worker.getString("id"),
                                sessionApi = sessionApi!!,
                                wireGuard = wireGuard!!
                            ),
                            sessionApi!!.priority + wireGuard!!.priority,
                            health
                        )
                    )
                }
            }
        }

        return candidates
            .sortedWith(
                compareBy<Triple<WorkerRoute, Int, Double>> { it.second }
                    .thenByDescending { it.third }
            )
            .firstOrNull()
            ?.first
            ?: error("No active WireGuard worker is available")
    }

    private fun parse(json: JSONObject): NetworkEndpoint =
        NetworkEndpoint(
            scheme = json.getString("scheme"),
            host = json.getString("host"),
            port = json.getInt("port"),
            path = json.optString("path", ""),
            priority = json.optInt("priority", 100)
        )
}
