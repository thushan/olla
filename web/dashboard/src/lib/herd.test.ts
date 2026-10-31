import { describe, it, expect, beforeEach, vi } from 'vitest';
import {
  COLUMNS_STORAGE_KEY,
  defaultColumns,
  sanitiseColumns,
  loadColumns,
  saveColumns,
  filterEndpoints,
  isFiltering,
  estimateFailures,
  parseProbeMs,
  issueList,
} from './herd';
import { parseTrafficBytes } from './format';

beforeEach(() => {
  localStorage.clear();
  vi.restoreAllMocks();
});

describe('column persistence', () => {
  it('defaults to In flight, Requests, Type and Models', () => {
    expect(loadColumns()).toEqual(['active_connections', 'request_count', 'type', 'model_count']);
  });

  it('round-trips a choice in canonical order', () => {
    saveColumns(['traffic_bytes', 'priority']);
    expect(loadColumns()).toEqual(['priority', 'traffic_bytes']);
  });

  it('keeps an explicit empty selection rather than reverting to defaults', () => {
    saveColumns([]);
    expect(loadColumns()).toEqual([]);
  });

  it('ignores unknown or stale keys', () => {
    expect(sanitiseColumns(['type', 'latency_range', 42, null, 'failures'])).toEqual(['type', 'failures']);
  });

  it('falls back to defaults on corrupt or wrongly shaped storage', () => {
    localStorage.setItem(COLUMNS_STORAGE_KEY, '{not json');
    expect(loadColumns()).toEqual(defaultColumns());
    localStorage.setItem(COLUMNS_STORAGE_KEY, '{"a":1}');
    expect(loadColumns()).toEqual(defaultColumns());
  });

  it('survives storage that throws', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked');
    });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('blocked');
    });
    expect(loadColumns()).toEqual(defaultColumns());
    expect(() => saveColumns(['type'])).not.toThrow();
  });
});

describe('filterEndpoints', () => {
  const rows = [
    { name: 'gpu-box', type: 'ollama', status: 'healthy' },
    { name: 'mac-mini', type: 'vllm-mlx', status: 'offline' },
    { name: 'Lab-2', type: 'vllm', status: 'degraded' },
  ];
  it('returns everything with no filter', () => {
    expect(filterEndpoints(rows, '  ', 'all')).toHaveLength(3);
  });
  it('matches name or type, case-insensitively', () => {
    expect(filterEndpoints(rows, 'LAB', 'all').map((r) => r.name)).toEqual(['Lab-2']);
    expect(filterEndpoints(rows, 'vllm', 'all').map((r) => r.name)).toEqual(['mac-mini', 'Lab-2']);
  });
  it('unhealthy keeps everything that is not healthy', () => {
    expect(filterEndpoints(rows, '', 'unhealthy').map((r) => r.name)).toEqual(['mac-mini', 'Lab-2']);
  });
  it('combines query and status', () => {
    expect(filterEndpoints(rows, 'ollama', 'unhealthy')).toEqual([]);
  });
  it('isFiltering reflects either control', () => {
    expect(isFiltering('', 'all')).toBe(false);
    expect(isFiltering(' x', 'all')).toBe(true);
    expect(isFiltering('', 'unhealthy')).toBe(true);
  });
});

describe('derived values', () => {
  it('estimates failures from requests and rate', () => {
    expect(estimateFailures(1000, 98.5)).toBe(15);
    expect(estimateFailures(0, 0)).toBeNull();
    expect(estimateFailures(10, 100)).toBe(0);
  });
  it('parses probe latency strings from pkg/format.Latency', () => {
    expect(parseProbeMs('12ms')).toBe(12);
    expect(parseProbeMs('1.5s')).toBe(1500);
    expect(parseProbeMs('')).toBeNull();
    expect(parseProbeMs(undefined)).toBeNull();
    expect(parseProbeMs('n/a')).toBeNull();
  });
  it('parses traffic strings back to bytes', () => {
    expect(parseTrafficBytes('21.09 KB')).toBeCloseTo(21.09 * 1024);
    expect(parseTrafficBytes('0 B')).toBe(0);
    expect(parseTrafficBytes('junk')).toBeNull();
  });
  it('splits issues', () => {
    expect(issueList('unavailable')).toEqual(['unavailable']);
    expect(issueList('a, b')).toEqual(['a', 'b']);
    expect(issueList('')).toEqual([]);
    expect(issueList(undefined)).toEqual([]);
  });
});
