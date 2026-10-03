# Billing invariants

Billing Core применяет дополнительные инварианты независимо от конкретного провайдера.

1. `amount_minor` и `currency` нормализованного события обязаны точно совпадать с указанной версией тарифа.
2. Provider payment навсегда привязан к одному `user_id + plan_id + amount + currency`.
3. Подписка продлевается только при **первом переходе конкретного provider payment в succeeded**.
4. Повторный `payment.succeeded`, другой provider event для уже succeeded payment или повторная доставка webhook не продлевают период второй раз.
5. Финальный succeeded не регрессирует в pending/cancelled при запоздавшем событии.
6. Refunded является финальным состоянием платежа для дальнейшей финансовой обработки.

Эти проверки находятся в общем Billing Core, а не только в YooKassa adapter, поэтому сохраняются при добавлении других эквайеров.
