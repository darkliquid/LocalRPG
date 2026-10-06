import { describe, expect, it } from 'vitest';
import { slugify } from './slug';

describe('slugify', () => {
  it('lowercases and hyphenates', () => {
    expect(slugify('Lady Evelyn Vance')).toBe('lady-evelyn-vance');
  });

  it('collapses runs of separators', () => {
    expect(slugify('the   quay--district')).toBe('the-quay-district');
  });

  it('drops leading and trailing separators', () => {
    expect(slugify('  _The Quay_ ')).toBe('the-quay');
  });

  it('strips punctuation', () => {
    expect(slugify("Sean O'Malley!")).toBe('sean-omalley');
  });

  it('keeps digits', () => {
    expect(slugify('Sector 7-G')).toBe('sector-7-g');
  });
});
