import { useEffect, useState } from 'react';
import { useTranslations } from 'use-intl';
import { Rocket, KeyRound, RefreshCw, Activity, Power } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import { useSidecarStatus, useSidecarConfig, useSetSidecarConfig, useStartSidecar, useStopSidecar } from '@/api/sidecar';
import { toast } from 'sonner';

// SettingSidecar 渲染 Zen 侧车配置卡片：配置块 + 状态块 + 生效语义说明。
export function SettingSidecar() {
    const t = useTranslations('setting');
    const { data: status } = useSidecarStatus();
    const { data: config } = useSidecarConfig();
    const setConfig = useSetSidecarConfig();
    const startSidecar = useStartSidecar();
    const stopSidecar = useStopSidecar();

    const [anonymous, setAnonymous] = useState(true);
    const [zenKeys, setZenKeys] = useState('');
    const [goKeys, setGoKeys] = useState('');
    const [prefer, setPrefer] = useState('zen');

    useEffect(() => {
        if (config) {
            if (typeof config['anonymous'] === 'boolean') {
                queueMicrotask(() => setAnonymous(config['anonymous'] as boolean));
            }
            const arr = (v: unknown) => Array.isArray(v) ? (v as unknown[]).filter(x => x !== '***').join(',') : '';
            queueMicrotask(() => {
                setZenKeys(arr(config['zen_keys']));
                setGoKeys(arr(config['go_keys']));
            });
            if (typeof config['prefer'] === 'string') {
                queueMicrotask(() => setPrefer(config['prefer'] as string));
            }
        }
    }, [config]);

    const save = (patch: Record<string, unknown>) => {
        setConfig.mutate(patch, {
            onSuccess: () => toast.success(t('saved')),
            onError: (e: unknown) => toast.error(e instanceof Error ? e.message : String(e)),
        });
    };

    const splitKeys = (s: string) =>
        s.split(/[,，\n]/).map(x => x.trim()).filter(Boolean);

    const running = status?.running ?? false;

    const toggleRuntime = (v: boolean) => {
        if (v) {
            startSidecar.mutate(undefined, {
                onSuccess: () => toast.success(t('sidecar.startedToast')),
                onError: (e: unknown) => toast.error(e instanceof Error ? e.message : String(e)),
            });
        } else {
            stopSidecar.mutate(undefined, {
                onSuccess: () => toast.success(t('sidecar.stoppedToast')),
                onError: (e: unknown) => toast.error(e instanceof Error ? e.message : String(e)),
            });
        }
    };
    const health = status?.health;
    const models = health?.models;
    const keys = health?.keys;

    return (
        <div className="rounded-3xl border border-border bg-card p-6 space-y-5">
            <h2 className="text-lg font-bold text-card-foreground flex items-center gap-2">
                <Rocket className="h-5 w-5" />
                {t('sidecar.title')}
            </h2>

            {!running && (
                <p className="text-sm text-muted-foreground">{t('sidecar.disabled')}</p>
            )}

            {/* 运行时开关：开即启动，关即停止；默认关闭，仅影响本轮运行 */}
            <div className="flex items-center justify-between gap-4">
                <div className="flex items-center gap-3">
                    <Power className="h-5 w-5 text-muted-foreground" />
                    <span className="text-sm font-medium">{t('sidecar.runtime')}</span>
                </div>
                <Switch
                    checked={running}
                    onCheckedChange={toggleRuntime}
                    disabled={startSidecar.isPending || stopSidecar.isPending}
                />
            </div>

            {running && (
                <>
                    {/* 运行状态 */}
                    <div className="flex items-center justify-between gap-4">
                        <div className="flex items-center gap-3">
                            <Activity className="h-5 w-5 text-muted-foreground" />
                            <span className="text-sm font-medium">{t('sidecar.status')}</span>
                        </div>
                        <span className="text-sm text-muted-foreground">
                            {status?.error ?? t('sidecar.running', { models: models?.exposed ?? 0, total: models?.total ?? 0 })}
                        </span>
                    </div>
                    {keys && (
                        <div className="text-xs text-muted-foreground text-right">
                            {t('sidecar.keys', { zen: keys.zen, go: keys.go, anonymous: keys.anonymous ? t('sidecar.on') : t('sidecar.off') })}
                        </div>
                    )}

                    {/* 匿名免费优先 */}
                    <div className="flex items-center justify-between gap-4">
                        <div className="flex items-center gap-3">
                            <span className="text-sm font-medium">{t('sidecar.anonymous')}</span>
                        </div>
                        <Switch
                            checked={anonymous}
                            onCheckedChange={(v) => { setAnonymous(v); save({ anonymous: v }); }}
                        />
                    </div>

                    {/* Zen Key */}
                    <div className="flex items-center justify-between gap-4">
                        <div className="flex items-center gap-3">
                            <KeyRound className="h-5 w-5 text-muted-foreground" />
                            <span className="text-sm font-medium">{t('sidecar.zenKeys')}</span>
                        </div>
                        <Input
                            type="password"
                            value={zenKeys}
                            onChange={(e) => setZenKeys(e.target.value)}
                            onBlur={() => save({ zen_keys: splitKeys(zenKeys) })}
                            placeholder={t('sidecar.zenKeysPlaceholder')}
                            className="w-48 rounded-xl"
                        />
                    </div>

                    {/* Go Key */}
                    <div className="flex items-center justify-between gap-4">
                        <div className="flex items-center gap-3">
                            <KeyRound className="h-5 w-5 text-muted-foreground" />
                            <span className="text-sm font-medium">{t('sidecar.goKeys')}</span>
                        </div>
                        <Input
                            type="password"
                            value={goKeys}
                            onChange={(e) => setGoKeys(e.target.value)}
                            onBlur={() => save({ go_keys: splitKeys(goKeys) })}
                            placeholder={t('sidecar.goKeysPlaceholder')}
                            className="w-48 rounded-xl"
                        />
                    </div>

                    {/* Tier 偏好 */}
                    <div className="flex items-center justify-between gap-4">
                        <div className="flex items-center gap-3">
                            <RefreshCw className="h-5 w-5 text-muted-foreground" />
                            <span className="text-sm font-medium">{t('sidecar.prefer')}</span>
                        </div>
                        <Input
                            value={prefer}
                            onChange={(e) => setPrefer(e.target.value)}
                            onBlur={() => save({ prefer })}
                            placeholder="zen"
                            className="w-48 rounded-xl"
                        />
                    </div>

                    <p className="text-xs text-muted-foreground">{t('sidecar.hint')}</p>
                </>
            )}
        </div>
    );
}
