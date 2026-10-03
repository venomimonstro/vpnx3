# Refund lifecycle

ЮKassa поддерживает webhook refund.succeeded и полный/частичный возврат. VPNX3 обрабатывает refund как отдельный financial object.

Правила:
- webhook refund.succeeded перепроверяется серверным GET /refunds/{id};
- затем отдельно загружается исходный payment и сверяются payment_id, user_id, plan_id и currency;
- каждый refund имеет собственный provider_refund_id и идемпотентность;
- частичный возврат уменьшает net revenue, но не меняет доступ автоматически;
- когда cumulative succeeded refunds == original payment amount, payment становится refunded;
- только полный возврат отзывает subscription_credit, созданный конкретным payment;
- credit отзывается один раз, срок subscription уменьшается на его billing period;
- если после уменьшения expires_at уже в прошлом, subscription становится expired.

Это избегает некорректного сценария «вернули 10 ₽ из 199 ₽ — отключили весь месяц».

Финансовая аналитика показывает captured, refunded и net за выбранный период.
