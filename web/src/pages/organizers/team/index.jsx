import React, { useCallback, useEffect, useState } from 'react';
import { z } from 'zod';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { SkeletonList } from '@/components/ui/skeleton';
import { EmptyState } from '@/components/ui/empty-state';
import { ErrorState } from '@/components/ui/error-state';
import { toast } from '@/components/ui/use-toast';
import { Users, Mail, Clock, X, ShieldAlert, UserPlus } from 'lucide-react';
import { useAuth } from '@/context/use-auth';
import { orgMembers as orgMembersApi } from '@/lib/api';

const ROLE_LABEL = { owner: 'Owner', admin: 'Admin', scanner: 'Scanner' };
const INVITABLE_ROLES = ['admin', 'scanner'];

const inviteSchema = z.object({
    email: z.string().trim().email('Enter a valid email address.'),
    role: z.enum(['admin', 'scanner']),
});

function formatExpiry(iso) {
    if (!iso) return null;
    try {
        return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(iso));
    } catch {
        return iso;
    }
}

const TeamPage = () => {
    const { activeOrg, user } = useAuth();
    const canManage = activeOrg?.role === 'owner' || activeOrg?.role === 'admin';

    const [state, setState] = useState({ members: [], invites: [], loading: true, error: null });
    const [inviting, setInviting] = useState(false);
    const [revokingId, setRevokingId] = useState(null);
    const [roleChangingId, setRoleChangingId] = useState(null);

    const form = useForm({ resolver: zodResolver(inviteSchema), defaultValues: { email: '', role: 'scanner' } });

    const load = useCallback(async () => {
        if (!activeOrg?.id || !canManage) {
            setState({ members: [], invites: [], loading: false, error: null });
            return;
        }
        setState((s) => ({ ...s, loading: true, error: null }));
        try {
            const [membersData, invitesData] = await Promise.all([
                orgMembersApi.list(activeOrg.id),
                orgMembersApi.invites(activeOrg.id),
            ]);
            const members = Array.isArray(membersData) ? membersData : (membersData?.members ?? []);
            const invites = Array.isArray(invitesData) ? invitesData : (invitesData?.invites ?? []);
            setState({ members, invites, loading: false, error: null });
        } catch (err) {
            setState({ members: [], invites: [], loading: false, error: err.message || 'Could not load your team.' });
        }
    }, [activeOrg?.id, canManage]);

    useEffect(() => {
        load();
    }, [load]);

    const handleInvite = async (data) => {
        setInviting(true);
        try {
            await orgMembersApi.invite(activeOrg.id, data);
            toast({ title: 'Invite sent', description: `${data.email} has been invited as ${ROLE_LABEL[data.role]}.` });
            form.reset({ email: '', role: 'scanner' });
            load();
        } catch (err) {
            toast({ title: 'Could not send invite', description: err.message, variant: 'destructive' });
        } finally {
            setInviting(false);
        }
    };

    const handleRevoke = async (inviteId) => {
        setRevokingId(inviteId);
        try {
            await orgMembersApi.revokeInvite(inviteId);
            setState((s) => ({ ...s, invites: s.invites.filter((i) => i.id !== inviteId && i.invite_id !== inviteId) }));
            toast({ title: 'Invite revoked' });
        } catch (err) {
            toast({ title: 'Could not revoke invite', description: err.message, variant: 'destructive' });
        } finally {
            setRevokingId(null);
        }
    };

    const handleRoleChange = async (member, role) => {
        setRoleChangingId(member.user_id);
        try {
            await orgMembersApi.updateRole(activeOrg.id, member.user_id, role);
            setState((s) => ({ ...s, members: s.members.map((m) => (m.user_id === member.user_id ? { ...m, role } : m)) }));
            toast({ title: 'Role updated', description: `${member.name || member.email} is now ${ROLE_LABEL[role]}.` });
        } catch (err) {
            if (err.status === 404 || err.status === 405) {
                toast({ title: 'Not available yet', description: 'Changing a member’s role isn’t wired up on this server build.' });
            } else {
                toast({ title: 'Could not update role', description: err.message, variant: 'destructive' });
            }
        } finally {
            setRoleChangingId(null);
        }
    };

    if (!canManage) {
        return (
            <div className="mx-auto max-w-2xl py-12">
                <ErrorState
                    icon={ShieldAlert}
                    title="Admin access required"
                    description="Only org owners and admins can manage the team. Ask an owner or admin to invite you at a higher role if you need this."
                />
            </div>
        );
    }

    return (
        <div className="mx-auto max-w-4xl space-y-6">
            <div className="flex items-center gap-3">
                <Users className="h-8 w-8 text-primary" />
                <div>
                    <h1 className="font-display text-3xl font-bold">Team</h1>
                    {activeOrg && <p className="text-sm text-muted-foreground">{activeOrg.name}</p>}
                </div>
            </div>

            <Card>
                <CardHeader>
                    <CardTitle className="flex items-center gap-2 text-base">
                        <UserPlus className="h-4 w-4" />
                        Invite someone
                    </CardTitle>
                    <CardDescription>They&apos;ll receive a link to join {activeOrg?.name}.</CardDescription>
                </CardHeader>
                <CardContent>
                    <Form {...form}>
                        <form onSubmit={form.handleSubmit(handleInvite)} className="flex flex-col gap-3 sm:flex-row sm:items-start">
                            <FormField
                                control={form.control}
                                name="email"
                                render={({ field }) => (
                                    <FormItem className="flex-1">
                                        <FormLabel className="sr-only">Email</FormLabel>
                                        <FormControl>
                                            <Input {...field} type="email" placeholder="teammate@example.com" disabled={inviting} />
                                        </FormControl>
                                        <FormMessage />
                                    </FormItem>
                                )}
                            />
                            <FormField
                                control={form.control}
                                name="role"
                                render={({ field }) => (
                                    <FormItem>
                                        <FormLabel className="sr-only">Role</FormLabel>
                                        <Select value={field.value} onValueChange={field.onChange} disabled={inviting}>
                                            <FormControl>
                                                <SelectTrigger className="w-full sm:w-40">
                                                    <SelectValue />
                                                </SelectTrigger>
                                            </FormControl>
                                            <SelectContent>
                                                {INVITABLE_ROLES.map((r) => (
                                                    <SelectItem key={r} value={r}>
                                                        {ROLE_LABEL[r]}
                                                    </SelectItem>
                                                ))}
                                            </SelectContent>
                                        </Select>
                                    </FormItem>
                                )}
                            />
                            <Button type="submit" disabled={inviting}>
                                {inviting ? 'Sending…' : 'Send invite'}
                            </Button>
                        </form>
                    </Form>
                </CardContent>
            </Card>

            <Card>
                <CardHeader>
                    <CardTitle className="text-base">Members</CardTitle>
                </CardHeader>
                <CardContent>
                    {state.loading ? (
                        <SkeletonList rows={3} />
                    ) : state.error ? (
                        <ErrorState description={state.error} onRetry={load} />
                    ) : state.members.length === 0 ? (
                        <EmptyState icon={Users} title="No members yet" description="Invite your team above." />
                    ) : (
                        <div className="overflow-x-auto">
                            <Table>
                                <TableHeader>
                                    <TableRow>
                                        <TableHead>Name</TableHead>
                                        <TableHead>Email</TableHead>
                                        <TableHead className="text-right">Role</TableHead>
                                    </TableRow>
                                </TableHeader>
                                <TableBody>
                                    {state.members.map((m) => {
                                        const isSelf = m.user_id === user?.id;
                                        return (
                                            <TableRow key={m.user_id}>
                                                <TableCell className="font-medium">
                                                    {m.name || '—'} {isSelf && <span className="text-muted-foreground">(you)</span>}
                                                </TableCell>
                                                <TableCell className="text-muted-foreground">{m.email}</TableCell>
                                                <TableCell className="text-right">
                                                    {m.role === 'owner' || isSelf ? (
                                                        <Badge variant="secondary" className="capitalize">
                                                            {ROLE_LABEL[m.role] ?? m.role}
                                                        </Badge>
                                                    ) : (
                                                        <Select
                                                            value={m.role}
                                                            onValueChange={(role) => handleRoleChange(m, role)}
                                                            disabled={roleChangingId === m.user_id}
                                                        >
                                                            <SelectTrigger className="ml-auto w-32">
                                                                <SelectValue />
                                                            </SelectTrigger>
                                                            <SelectContent>
                                                                {INVITABLE_ROLES.map((r) => (
                                                                    <SelectItem key={r} value={r}>
                                                                        {ROLE_LABEL[r]}
                                                                    </SelectItem>
                                                                ))}
                                                            </SelectContent>
                                                        </Select>
                                                    )}
                                                </TableCell>
                                            </TableRow>
                                        );
                                    })}
                                </TableBody>
                            </Table>
                        </div>
                    )}
                </CardContent>
            </Card>

            <Card>
                <CardHeader>
                    <CardTitle className="text-base">Pending invites</CardTitle>
                </CardHeader>
                <CardContent>
                    {state.loading ? (
                        <SkeletonList rows={2} />
                    ) : state.invites.length === 0 ? (
                        <p className="text-sm text-muted-foreground">No pending invites.</p>
                    ) : (
                        <ul className="divide-y divide-border">
                            {state.invites.map((inv) => {
                                const inviteId = inv.id ?? inv.invite_id;
                                return (
                                    <li key={inviteId} className="flex items-center justify-between gap-3 py-3">
                                        <div className="min-w-0">
                                            <p className="flex items-center gap-1.5 truncate text-sm font-medium">
                                                <Mail className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                                                {inv.email}
                                            </p>
                                            <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
                                                <Badge variant="outline" className="capitalize">
                                                    {ROLE_LABEL[inv.role] ?? inv.role}
                                                </Badge>
                                                {inv.expires_at && (
                                                    <span className="flex items-center gap-1">
                                                        <Clock className="h-3 w-3" />
                                                        Expires {formatExpiry(inv.expires_at)}
                                                    </span>
                                                )}
                                            </p>
                                        </div>
                                        <Button
                                            variant="ghost"
                                            size="sm"
                                            onClick={() => handleRevoke(inviteId)}
                                            disabled={revokingId === inviteId}
                                            className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                                        >
                                            <X className="mr-1.5 h-4 w-4" />
                                            Revoke
                                        </Button>
                                    </li>
                                );
                            })}
                        </ul>
                    )}
                </CardContent>
            </Card>
        </div>
    );
};

export default TeamPage;
