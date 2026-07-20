import React from 'react';
import { useAuth } from '@/context/use-auth';
import { useTheme } from '@/components/theme-provider';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Building2, Mail, Moon, Sun, Monitor, LogOut, Users, Banknote, ChevronRight } from 'lucide-react';
import { useNavigate } from 'react-router-dom';

const THEME_OPTIONS = [
    { value: 'light', label: 'Light', icon: Sun },
    { value: 'dark', label: 'Dark', icon: Moon },
    { value: 'system', label: 'System', icon: Monitor },
];

const SettingsPage = () => {
    const { user, orgs, signOut } = useAuth();
    const { theme, setTheme } = useTheme();
    const navigate = useNavigate();

    const handleSignOut = async () => {
        await signOut();
        navigate('/login');
    };

    return (
        <div className="mx-auto max-w-3xl space-y-6">
            <h1 className="font-display text-3xl font-bold">Settings</h1>

            <Card>
                <CardHeader>
                    <CardTitle>Account</CardTitle>
                    <CardDescription>Your Cackle account</CardDescription>
                </CardHeader>
                <CardContent className="space-y-3">
                    <div className="flex items-center gap-3 text-sm">
                        <Mail className="h-4 w-4 text-muted-foreground" />
                        <span>{user?.email}</span>
                    </div>
                    {user?.name && <p className="pl-7 text-sm text-muted-foreground">{user.name}</p>}
                </CardContent>
            </Card>

            <Card>
                <CardHeader>
                    <CardTitle>Organizations</CardTitle>
                    <CardDescription>Organizations you belong to</CardDescription>
                </CardHeader>
                <CardContent>
                    {orgs.length === 0 ? (
                        <p className="text-sm text-muted-foreground">You&apos;re not part of an organization yet.</p>
                    ) : (
                        <div className="divide-y divide-border rounded-md border border-border">
                            {orgs.map((org) => (
                                <div key={org.id} className="flex items-center justify-between p-4">
                                    <div className="flex items-center gap-3">
                                        <div className="rounded-full bg-muted p-2">
                                            <Building2 className="h-4 w-4 text-muted-foreground" />
                                        </div>
                                        <span className="font-medium">{org.name}</span>
                                    </div>
                                    <Badge variant="secondary" className="capitalize">
                                        {org.role}
                                    </Badge>
                                </div>
                            ))}
                        </div>
                    )}
                </CardContent>
            </Card>

            <Card>
                <CardHeader>
                    <CardTitle>Manage your org</CardTitle>
                    <CardDescription>Team access and where your payouts land.</CardDescription>
                </CardHeader>
                <CardContent className="divide-y divide-border rounded-md border border-border p-0">
                    <button
                        type="button"
                        onClick={() => navigate('/admin/team')}
                        className="flex w-full items-center justify-between p-4 text-left transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                    >
                        <span className="flex items-center gap-3">
                            <Users className="h-4 w-4 text-muted-foreground" />
                            <span>
                                <span className="block font-medium">Team</span>
                                <span className="block text-sm text-muted-foreground">Members, roles, and invites</span>
                            </span>
                        </span>
                        <ChevronRight className="h-4 w-4 text-muted-foreground" />
                    </button>
                    <button
                        type="button"
                        onClick={() => navigate('/admin/payouts')}
                        className="flex w-full items-center justify-between p-4 text-left transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                    >
                        <span className="flex items-center gap-3">
                            <Banknote className="h-4 w-4 text-muted-foreground" />
                            <span>
                                <span className="block font-medium">Payouts</span>
                                <span className="block text-sm text-muted-foreground">Bank account and per-event payout totals</span>
                            </span>
                        </span>
                        <ChevronRight className="h-4 w-4 text-muted-foreground" />
                    </button>
                </CardContent>
            </Card>

            <Card>
                <CardHeader>
                    <CardTitle>Appearance</CardTitle>
                    <CardDescription>Choose how Cackle looks on this device</CardDescription>
                </CardHeader>
                <CardContent>
                    <div className="flex gap-2">
                        {THEME_OPTIONS.map(({ value, label, icon: Icon }) => (
                            <Button key={value} variant={theme === value ? 'default' : 'outline'} size="sm" onClick={() => setTheme(value)}>
                                <Icon className="mr-2 h-4 w-4" />
                                {label}
                            </Button>
                        ))}
                    </div>
                </CardContent>
            </Card>

            <Card>
                <CardContent className="flex items-center justify-between p-6">
                    <div>
                        <p className="font-medium">Sign out</p>
                        <p className="text-sm text-muted-foreground">End your session on this device.</p>
                    </div>
                    <Button variant="destructive" onClick={handleSignOut}>
                        <LogOut className="mr-2 h-4 w-4" />
                        Sign Out
                    </Button>
                </CardContent>
            </Card>
        </div>
    );
};

export default SettingsPage;
