# Node lifecycle

После enrollment и первого authenticated heartbeat нода автоматически проходит:

`enrolling -> provisioning -> testing`

Дальше требуется операторское решение:

1. `POST /api/v1/nodes/{id}/approve`:
   `testing -> draft`
2. `POST /api/v1/nodes/{id}/publish`:
   `draft -> active`

Это соответствует существующей state machine и предотвращает прямой переход новой машины из автоматического testing в production.

Build workers также обязаны пройти оба шага. До status=active очередь сборки задания им не выдаёт.
