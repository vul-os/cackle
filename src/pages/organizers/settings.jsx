import React, { useState, useEffect, useContext } from 'react';
import { AuthContext } from '@/context/use-auth';
import { supabase } from '@/services/supabaseClient';
import { 
  Users, 
  Shield,
  UserPlus,
  Mail,
  X,
  Loader2,
} from 'lucide-react';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { 
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import { toast } from 'sonner';

const AVAILABLE_ROLES = ['admin', 'viewer'];

const InviteMemberDialog = ({ open, onOpenChange, onInvite, isLoading }) => {
  const [email, setEmail] = useState('');
  const [selectedRole, setSelectedRole] = useState('');

  const handleSubmit = async (e) => {
    e.preventDefault();
    const success = await onInvite(email, selectedRole);
    if (success) {
      setEmail('');
      setSelectedRole('');
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Invite New Member</DialogTitle>
          <DialogDescription>
            Send an invitation to join your organization
          </DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit}>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <label className="text-sm font-medium">Email</label>
              <Input
                placeholder="Email address"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
              />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">Role</label>
              <Select
                value={selectedRole}
                onValueChange={setSelectedRole}
                required
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select a role" />
                </SelectTrigger>
                <SelectContent>
                  {AVAILABLE_ROLES.map((role) => (
                    <SelectItem key={role} value={role}>
                      {role.charAt(0).toUpperCase() + role.slice(1)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" type="button" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={isLoading || !email || !selectedRole}>
              {isLoading ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Sending...
                </>
              ) : (
                'Send Invitation'
              )}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
};

const SettingsPage = () => {
  const { user, activeOrganization } = useContext(AuthContext);
  const [members, setMembers] = useState([]);
  const [invitations, setInvitations] = useState([]);
  const [loading, setLoading] = useState(true);
  const [inviteLoading, setInviteLoading] = useState(false);
  const [inviteDialogOpen, setInviteDialogOpen] = useState(false);

  const fetchData = async () => {
    if (!activeOrganization) return;

    try {
      setLoading(true);
      const [membersData, invitationsData] = await Promise.all([
        supabase
          .from('organization_members')
          .select(`
            id,
            role,
            profiles:profile_id (email, full_name)
          `)
          .eq('organization_id', activeOrganization.id),
        supabase.rpc('get_organization_pending_invitations', {
          p_organization_id: activeOrganization.id
        }),
      ]);

      if (membersData.error) throw membersData.error;
      if (invitationsData.error) throw invitationsData.error;

      setMembers(membersData.data || []);
      setInvitations(invitationsData.data || []);
    } catch (error) {
      console.error('Error fetching data:', error);
      toast.error('Failed to load data');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
    
    if (activeOrganization) {
      // Subscribe to all relevant tables
      const channel = supabase
        .channel(`org-changes-${activeOrganization.id}`)
        .on('postgres_changes', {
          event: '*',
          schema: 'public',
          table: 'organization_members',
          filter: `organization_id=eq.${activeOrganization.id}`
        }, () => fetchData())
        .on('postgres_changes', {
          event: '*',
          schema: 'public',
          table: 'organization_invitations',
          filter: `organization_id=eq.${activeOrganization.id}`
        }, () => fetchData())
        .subscribe();

      return () => {
        supabase.removeChannel(channel);
      };
    }
  }, [activeOrganization]);

  const handleInviteMember = async (email, role) => {
    setInviteLoading(true);
    try {
      const { error } = await supabase.functions.invoke('invite-user', {
        body: {
          email,
          organization_id: activeOrganization.id,
          role
        }
      });

      if (error) throw error;
      
      toast.success(`Invitation sent to ${email}`);
      setInviteDialogOpen(false);
      
      // Manually refresh data after successful invitation
      await fetchData();
      return true;
    } catch (error) {
      console.error('Error inviting member:', error);
      toast.error(error.message || 'Failed to send invitation');
      return false;
    } finally {
      setInviteLoading(false);
    }
  };

  const handleCancelInvitation = async (invitationId) => {
    try {
      const { error } = await supabase.rpc(
        'cancel_organization_invitation',
        { p_invitation_id: invitationId }
      );

      if (error) throw error;
      
      toast.success('Invitation cancelled');
      
      // Manually refresh data after successful cancellation
      await fetchData();
      
      // Update local state immediately for better UX
      setInvitations(prev => prev.filter(inv => inv.id !== invitationId));
    } catch (error) {
      console.error('Error cancelling invitation:', error);
      toast.error(error.message || 'Failed to cancel invitation');
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-96">
        <Loader2 className="h-8 w-8 animate-spin" />
      </div>
    );
  }

  return (
    <div className="container mx-auto py-8 max-w-6xl">
      <Card>
        <CardHeader>
          <CardTitle>Members</CardTitle>
          <CardDescription>
            Manage your organization members and their roles
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-6">
            <div className="flex justify-end">
              <InviteMemberDialog
                open={inviteDialogOpen}
                onOpenChange={setInviteDialogOpen}
                onInvite={handleInviteMember}
                isLoading={inviteLoading}
              />
              <Button onClick={() => setInviteDialogOpen(true)}>
                <UserPlus className="mr-2 h-4 w-4" />
                Invite Member
              </Button>
            </div>

            {/* Pending Invitations */}
            {invitations.length > 0 && (
              <div className="rounded-md border">
                <div className="bg-muted px-4 py-2 border-b">
                  <h3 className="text-sm font-medium">Pending Invitations</h3>
                </div>
                <div className="divide-y">
                  {invitations.map((invitation) => (
                    <div
                      key={invitation.id}
                      className="flex items-center justify-between p-4"
                    >
                      <div className="flex items-center space-x-4">
                        <div className="h-10 w-10 rounded-full bg-gray-100 flex items-center justify-center">
                          <Mail className="h-5 w-5 text-gray-500" />
                        </div>
                        <div>
                          <p className="font-medium">{invitation.email}</p>
                          <p className="text-sm text-gray-500">
                            {invitation.role.charAt(0).toUpperCase() + invitation.role.slice(1)} • Invited by {invitation.invited_by_name}
                          </p>
                        </div>
                      </div>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleCancelInvitation(invitation.id)}
                      >
                        <X className="h-4 w-4" />
                      </Button>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {/* Members List */}
            <div className="rounded-md border">
              <div className="divide-y">
                {members.map((member) => (
                  <div
                    key={member.id}
                    className="flex items-center justify-between p-4"
                  >
                    <div className="flex items-center space-x-4">
                      <div className="h-10 w-10 rounded-full bg-gray-100 flex items-center justify-center">
                        <Users className="h-5 w-5 text-gray-500" />
                      </div>
                      <div>
                        <p className="font-medium">
                          {member.profiles.full_name || member.profiles.email}
                        </p>
                        <p className="text-sm text-gray-500">
                          {member.profiles.email}
                        </p>
                      </div>
                    </div>
                    <div className="flex items-center space-x-2">
                      <div className="flex items-center space-x-1">
                        <Shield className="h-4 w-4 text-gray-500" />
                        <span className="text-sm text-gray-500">
                          {member.role.charAt(0).toUpperCase() + member.role.slice(1)}
                        </span>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
};

export default SettingsPage;