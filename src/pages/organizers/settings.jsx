import React, { useState, useEffect, useContext } from 'react';
import { AuthContext } from '@/context/use-auth';
import { supabase } from '@/services/supabaseClient';
import { 
  Users, 
  MoreVertical,
  Shield,
  UserPlus
} from 'lucide-react';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { toast } from 'sonner';

const SettingsPage = () => {
  const { user, activeOrganization } = useContext(AuthContext);
  const [members, setMembers] = useState([]);
  const [loading, setLoading] = useState(true);
  const [inviteEmail, setInviteEmail] = useState('');
  const [inviteDialogOpen, setInviteDialogOpen] = useState(false);

  // Fetch organization members
  const fetchMembers = async () => {
    if (!activeOrganization) return;
    
    try {
      const { data, error } = await supabase
        .from('organization_members')
        .select(`
          *,
          profiles:profile_id (
            email,
            full_name
          )
        `)
        .eq('organization_id', activeOrganization.id);

      if (error) throw error;
      setMembers(data || []);
    } catch (error) {
      console.error('Error fetching members:', error);
      toast.error('Failed to load members');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchMembers();
    
    // Set up real-time subscription
    if (activeOrganization) {
      const subscription = supabase
        .channel('org-members-changes')
        .on('postgres_changes', 
          { 
            event: '*', 
            schema: 'public', 
            table: 'organization_members',
            filter: `organization_id=eq.${activeOrganization.id}`
          }, 
          () => {
            fetchMembers();
          }
        )
        .subscribe();

      return () => {
        subscription.unsubscribe();
      };
    }
  }, [activeOrganization]);

  const handleInviteMember = async () => {
    if (!inviteEmail.trim() || !activeOrganization) return;

    try {
      // In a real application, you would:
      // 1. Check if the user exists in your system
      // 2. Send an invitation email
      // 3. Create a pending invitation record
      // For now, we'll just show a success message
      toast.success(`Invitation sent to ${inviteEmail}`);
      setInviteEmail('');
      setInviteDialogOpen(false);
    } catch (error) {
      console.error('Error inviting member:', error);
      toast.error('Failed to send invitation');
    }
  };

  const handleUpdateRole = async (memberId, newRole) => {
    try {
      const { error } = await supabase
        .from('organization_members')
        .update({ role: newRole })
        .eq('id', memberId);

      if (error) throw error;
      toast.success('Member role updated');
    } catch (error) {
      console.error('Error updating role:', error);
      toast.error('Failed to update role');
    }
  };

  const handleRemoveMember = async (memberId) => {
    try {
      const { error } = await supabase
        .from('organization_members')
        .delete()
        .eq('id', memberId);

      if (error) throw error;
      toast.success('Member removed');
    } catch (error) {
      console.error('Error removing member:', error);
      toast.error('Failed to remove member');
    }
  };

  const getRoleLabel = (role) => {
    switch (role) {
      case 'owner':
        return 'Owner';
      case 'admin':
        return 'Admin';
      case 'member':
        return 'Member';
      default:
        return 'Unknown';
    }
  };

  return (
    <div className="container mx-auto bg-white py-8 max-w-6xl">
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <h1 className="text-3xl font-bold">Settings</h1>
        </div>

        {/* Members Section */}
        <Card>
          <CardHeader className="flex flex-row items-center justify-between">
            <div>
              <CardTitle>Members</CardTitle>
              <CardDescription>
                Manage your organization members and their roles
              </CardDescription>
            </div>
            <Dialog open={inviteDialogOpen} onOpenChange={setInviteDialogOpen}>
              <DialogTrigger asChild>
                <Button>
                  <UserPlus className="mr-2 h-4 w-4" />
                  Invite Member
                </Button>
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>Invite New Member</DialogTitle>
                  <DialogDescription>
                    Send an invitation to join your organization
                  </DialogDescription>
                </DialogHeader>
                <div className="py-4">
                  <Input
                    placeholder="Email address"
                    type="email"
                    value={inviteEmail}
                    onChange={(e) => setInviteEmail(e.target.value)}
                  />
                </div>
                <DialogFooter>
                  <Button variant="outline" onClick={() => setInviteDialogOpen(false)}>
                    Cancel
                  </Button>
                  <Button onClick={handleInviteMember}>
                    Send Invitation
                  </Button>
                </DialogFooter>
              </DialogContent>
            </Dialog>
          </CardHeader>
          <CardContent>
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
                          {getRoleLabel(member.role)}
                        </span>
                      </div>
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="ghost" size="sm">
                            <MoreVertical className="h-4 w-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem
                            onClick={() => handleUpdateRole(member.id, 'admin')}
                          >
                            Make Admin
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            onClick={() => handleUpdateRole(member.id, 'member')}
                          >
                            Make Member
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            className="text-red-600"
                            onClick={() => handleRemoveMember(member.id)}
                          >
                            Remove Member
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
};

export default SettingsPage;