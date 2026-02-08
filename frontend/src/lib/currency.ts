export type CurrencyInfo = {
  code: string;
  symbol: string;
  exponent: number;
  flag: string;
};

export const COMMON_CURRENCIES: CurrencyInfo[] = [
  { code: 'USD', symbol: '$', exponent: 2, flag: '🇺🇸' },
  { code: 'EUR', symbol: '€', exponent: 2, flag: '🇪🇺' },
  { code: 'GBP', symbol: '£', exponent: 2, flag: '🇬🇧' },
  { code: 'JPY', symbol: '¥', exponent: 0, flag: '🇯🇵' },
  { code: 'CAD', symbol: '$', exponent: 2, flag: '🇨🇦' },
  { code: 'AUD', symbol: '$', exponent: 2, flag: '🇦🇺' },
  { code: 'CHF', symbol: 'Fr', exponent: 2, flag: '🇨🇭' },
  { code: 'CNY', symbol: '¥', exponent: 2, flag: '🇨🇳' },
  { code: 'KRW', symbol: '₩', exponent: 0, flag: '🇰🇷' },
  { code: 'MXN', symbol: '$', exponent: 2, flag: '🇲🇽' },
  { code: 'SGD', symbol: '$', exponent: 2, flag: '🇸🇬' },
  { code: 'HKD', symbol: '$', exponent: 2, flag: '🇭🇰' },
  { code: 'INR', symbol: '₹', exponent: 2, flag: '🇮🇳' },
  { code: 'SEK', symbol: 'kr', exponent: 2, flag: '🇸🇪' },
  { code: 'NOK', symbol: 'kr', exponent: 2, flag: '🇳🇴' }
];

export const EXPONENTS: Record<string, number> = COMMON_CURRENCIES.reduce((acc, c) => {
  acc[c.code] = c.exponent;
  return acc;
}, {} as Record<string, number>);

export const SYMBOLS: Record<string, string> = COMMON_CURRENCIES.reduce((acc, c) => {
  acc[c.code] = c.symbol;
  return acc;
}, {} as Record<string, string>);

export const FLAGS: Record<string, string> = COMMON_CURRENCIES.reduce((acc, c) => {
  acc[c.code] = c.flag;
  return acc;
}, {} as Record<string, string>);

export const DEFAULT_CURRENCY = 'USD';
