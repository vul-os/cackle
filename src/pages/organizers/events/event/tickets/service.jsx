import { supabase } from '@/services/supabaseClient';

export const ticketTypeService = {
  async fetchTicketTypes(eventId) {
    const { data, error } = await supabase
      .from('ticket_types')
      .select('*')
      .eq('event_id', eventId)
      .order('created_at', { ascending: false });

    if (error) throw error;
    return data;
  },

  async createTicketType(ticketTypeData) {
    const { data, error } = await supabase
      .from('ticket_types')
      .insert([ticketTypeData]);

    if (error) throw error;
    return data;
  },

  async updateTicketType(id, ticketTypeData) {
    const { data, error } = await supabase
      .from('ticket_types')
      .update(ticketTypeData)
      .eq('id', id);

    if (error) throw error;
    return data;
  },

  async deleteTicketType(id) {
    const { error } = await supabase
      .from('ticket_types')
      .delete()
      .eq('id', id);

    if (error) throw error;
  }
};