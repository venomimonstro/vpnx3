# Telemetry nullable worker key

Первичная версия агрегатной таблицы включала nullable worker_node_id в PRIMARY KEY. PostgreSQL делает все столбцы PRIMARY KEY NOT NULL, поэтому события без конкретного worker не могли агрегироваться.

Миграция 000015 исправляет это без удаления FK:

- worker_node_id остаётся nullable UUID/FK для аналитики;
- worker_node_key — строковый стабильный ключ агрегации;
- для событий без worker используется пустая строка;
- ON CONFLICT использует worker_node_key.

История миграций не переписывается.
