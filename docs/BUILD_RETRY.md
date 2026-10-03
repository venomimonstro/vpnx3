# Build retry policy

Failed/cancelled build job можно вернуть в очередь только явным административным действием.

Retry:

- не создаёт новый release;
- сохраняет исходный source commit и target;
- очищает прежнего build_worker;
- сбрасывает timestamps/error;
- возвращает release в status=building;
- пишет действие в audit log.

Running и succeeded jobs повторно поставить в очередь нельзя. Это защищает от случайной повторной публикации уже полученного артефакта.

Для artifact upload используется отдельный HTTP client без глобального 30-секундного timeout. Верхнюю границу времени задаёт context конкретной build-задачи, поэтому крупный AAB на медленном канале не обрывается через 30 секунд, но всё равно ограничен общим build timeout.
