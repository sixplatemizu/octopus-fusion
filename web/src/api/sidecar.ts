import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiRequest } from './client';

/**
 * Zen 侧车状态（经 octopus 后端反代，不直连侧车）。
 */
export interface SidecarStatus {
    enabled: boolean;
    running: boolean;
    error?: string;
    health?: {
        status: string;
        ready: boolean;
        version: string;
        models?: { total: number; exposed: number; zen: number; go: number };
        keys?: { zen: number; go: number; total: number; anonymous: boolean };
    };
}

export function useSidecarStatus() {
    return useQuery({
        queryKey: ['sidecar', 'status'],
        queryFn: () => apiRequest<SidecarStatus>('/api/v1/sidecar/status'),
        refetchInterval: 15000,
        refetchOnMount: 'always',
    });
}

export function useSidecarConfig() {
    return useQuery({
        queryKey: ['sidecar', 'config'],
        queryFn: () => apiRequest<Record<string, unknown>>('/api/v1/sidecar/config'),
        refetchOnMount: 'always',
    });
}

export function useSetSidecarConfig() {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: (data: Record<string, unknown>) =>
            apiRequest<Record<string, unknown>>('/api/v1/sidecar/config', { method: 'POST', body: data }),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['sidecar', 'config'] });
            queryClient.invalidateQueries({ queryKey: ['sidecar', 'status'] });
        },
    });
}

// useStartSidecar 运行时拉起：仅影响本轮运行，不改配置文件。
export function useStartSidecar() {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: () => apiRequest<Record<string, unknown>>('/api/v1/sidecar/start', { method: 'POST' }),
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['sidecar', 'status'] }),
    });
}

// useStopSidecar 运行时停止：重启后以配置文件 enabled 为准。
export function useStopSidecar() {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: () => apiRequest<Record<string, unknown>>('/api/v1/sidecar/stop', { method: 'POST' }),
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['sidecar', 'status'] }),
    });
}
