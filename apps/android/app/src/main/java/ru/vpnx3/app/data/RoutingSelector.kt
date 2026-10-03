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

private data class RouteCandidate(
    val route: WorkerRoute,
    val priority: Int,
    val latencyMs: Double,
    val health: Double
)

object RoutingSelector {
    fun candidates(config: VerifiedConfig): List<WorkerRoute> {
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
                    val latency = if (worker.has("latency_ms") && !worker.isNull("latency_ms")) {
                        worker.getDouble("latency_ms")
                    } else {
                        Double.MAX_VALUE
                    }
                    add(
                        RouteCandidate(
                            route = WorkerRoute(
                                nodeId = worker.getString("id"),
                                sessionApi = sessionApi!!,
                                wireGuard = wireGuard!!
                            ),
                            priority = sessionApi!!.priority + wireGuard!!.priority,
                            latencyMs = latency,
                            health = health
                        )
                    )
                }
            }
        }

        return candidates
            .sortedWith(
                compareBy<RouteCandidate> { it.priority }
                    .thenBy { it.latencyMs }
                    .thenByDescending { it.health }
            )
            .map { it.route }
    }

    fun select(config: VerifiedConfig): WorkerRoute =
        candidates(config).firstOrNull()
            ?: error("No active WireGuard worker is available")

    private fun parse(json: JSONObject): NetworkEndpoint =
        NetworkEndpoint(
            scheme = json.getString("scheme"),
            host = json.getString("host"),
            port = json.getInt("port"),
            path = json.optString("path", ""),
            priority = json.optInt("priority", 100)
        )
}
