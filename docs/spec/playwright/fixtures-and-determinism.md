# Playwright — Fixtures & Determinism (детерминизм скриншотов)

> Без детерминизма снятия скриншота visual regression бесполезен: diff будет шуметь на случайных пикселях,
> команда устанет от ложных срабатываний и отключит инструмент. Это «deterministic kit» — обязательная база.

## Принцип

Скриншот должен зависеть ТОЛЬКО от кода/верстки, которую мы тестируем, и ни от чего больше. Всё, что может
меняться от прогона к прогону при неизменном коде, должно быть зафиксировано или замаскировано:

- время и даты (часы, «сегодня», относительное время «5 минут назад»);
- анимации и переходы (могут попасть в кадр на разной фазе);
- локаль, таймзона, формат чисел/дат;
- шрифты (отсутствие шрифта → фолбэк → другой рендер);
- случайные данные (id, токены, рандомный контент);
- сетевые задержки (контент не догрузился к моменту снимка).

## Слой 1: конфиг проекта (локаль, TZ, viewport)

```ts
// playwright.config.ts — use
use: {
  locale: 'ru-RU',
  timezoneId: 'Europe/Moscow',
  viewport: { width: 1280, height: 720 },
  deviceScaleFactor: 1,            // фикс. DPR — иначе разные размеры PNG
  colorScheme: 'light',           // фикс. тема (если не тестируем dark отдельно)
}
```

> Фикс. `deviceScaleFactor` критичен: разный DPR → разный размер картинки → ложный CHANGED.

## Слой 2: общая фикстура детерминизма

Расширяем базовый `test` один раз; в тестах — ничего лишнего.

```ts
// fixtures/deterministic.ts
import { test as base, expect } from '@playwright/test';

export const test = base.extend({
  page: async ({ page }, use) => {
    // 1. Заморозить часы на фиксированной точке.
    await page.clock.install({ time: new Date('2025-01-01T12:00:00Z') });

    // 2. Отключить анимации/переходы и эффекты, влияющие на кадр.
    await page.addStyleTag({
      content: `
        *, *::before, *::after {
          animation-duration: 0s !important;
          animation-delay: 0s !important;
          transition-duration: 0s !important;
          transition-delay: 0s !important;
          caret-color: transparent !important;  /* мигающий курсор */
          scroll-behavior: auto !important;
        }
      `,
    });

    // 3. (опц.) Фиксировать Math.random / Date, если приложение их использует для контента.
    await page.addInitScript(() => {
      // пример: детерминированный Math.random
      let seed = 42;
      Math.random = () => { seed = (seed * 1103515245 + 12345) & 0x7fffffff; return seed / 0x7fffffff; };
    });

    await use(page);
  },
});

export { expect };
```

В тестах импортируем `test`/`expect` из этой фикстуры, а не из `@playwright/test`.

## Слой 3: маски динамических областей

Для областей, которые нельзя детерминировать (живой таймстемп, аватар из внешнего сервиса), — маскируем при
снимке. Playwright поддерживает `mask` нативно:

```ts
await expect(page).toHaveScreenshot('dashboard.png', {
  mask: [
    page.locator('[data-testid="last-updated"]'),
    page.locator('.live-clock'),
  ],
  // порог можно задать здесь или централизованно в Pixela per-project
  maxDiffPixelRatio: 0,
});
```

> В Pixela ignore-области (F-45) — серверный аналог/дополнение: маски можно задавать и в дашборде поверх
> снапшота, чтобы не править тесты. Но базовые, известные заранее динамические зоны лучше маскировать в тесте.

## Слой 4: ожидание стабильного состояния перед снимком

```ts
await page.goto('/events');
await page.waitForLoadState('networkidle');          // сеть утихла
await page.locator('.event-card').first().waitFor(); // ключевой контент появился
await page.evaluate(() => document.fonts.ready);      // шрифты загружены (иначе фолбэк-рендер)
await expect(page).toHaveScreenshot('events-list--desktop.png');
```

## Слой 5: шрифты в CI

- Использовать тот же базовый образ с теми же шрифтами в CI и локально (официальный
  `mcr.microsoft.com/playwright` фиксирует окружение).
- При кастомных шрифтах — устанавливать их в образ детерминированно, ждать `document.fonts.ready`.
- Несовпадение шрифтов CI vs локально — частая причина «у меня проходит, в CI нет».

## Чек-лист детерминизма (свериться перед тем, как доверять baseline)

- [ ] Зафиксированы locale, timezoneId, viewport, deviceScaleFactor, colorScheme.
- [ ] Часы заморожены (`page.clock`), относительное время стабильно.
- [ ] Анимации/переходы/каретка отключены глобально.
- [ ] Math.random/Date зафиксированы, если влияют на контент.
- [ ] Динамические области замаскированы (mask) или детерминированы.
- [ ] Перед снимком: networkidle + ожидание ключевого контента + fonts.ready.
- [ ] Один и тот же образ/шрифты в CI и локально.

## Связь с серверным diff

Детерминизм СНЯТИЯ (этот файл) и детерминизм СРАВНЕНИЯ (`specs/07-diff-engine.md`) — две независимые
необходимости. Первый обеспечивает, что одинаковый код → одинаковый PNG. Второй — что одинаковые PNG →
одинаковый результат diff. Нужны оба, иначе инструмент «флачит» и теряет доверие.
