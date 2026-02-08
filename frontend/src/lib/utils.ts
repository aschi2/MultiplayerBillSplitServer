export const initialsFromName = (name: string): string => {
  const parts = name.trim().split(/\s+/);
  if (parts.length === 0) return '?';
  if (parts.length === 1) return parts[0][0]?.toUpperCase() ?? '?';
  return `${parts[0][0]}${parts[parts.length - 1][0]}`.toUpperCase();
};

export const colorFromSeed = (seed: string): string => `#${seed.slice(0, 6)}`;

export const formatCurrency = (minorUnits: number, code = 'USD', symbol = '$', exponent = 2): string => {
  const factor = Math.pow(10, exponent);
  const value = minorUnits / factor;
  const fixed = value.toFixed(exponent);
  const prefix = symbol || code;
  return `${prefix}${fixed}`;
};
