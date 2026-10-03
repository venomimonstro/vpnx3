# Migration 000018 consolidation

В истории разработки одновременно появились две миграции с номером 000018:
- device_link_codes;
- device_pairing_codes.

Мигратор VPNX3 считает применённость только по numeric version, поэтому две миграции с одним номером недопустимы.

Исправление:
- legacy device_link runtime API удалён;
- основной контур — device_pairing_codes + account/status;
- legacy 000018_device_link_codes.sql удаляется из repository;
- 000019_device_pairing_repair.sql повторно создаёт pairing table идемпотентно и удаляет legacy table.

Это покрывает обе ситуации:
1. fresh install — 000018 pairing применяется, 000019 лишь подтверждает схему;
2. база успела применить старую 000018 link migration — 000019 создаёт правильную pairing table и удаляет legacy table.
