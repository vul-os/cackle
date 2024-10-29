import React, { useEffect, useState, useContext } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { supabase } from '../services/supabaseClient';
import { AuthContext } from '../context/use-auth';
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from "@/components/ui/accordion";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Folder, Save, ArrowUp, ArrowDown, Edit, Trash2 } from 'lucide-react';
import { toast } from "@/components/ui/use-toast";

const Items = () => {
  const [documentGroups, setDocumentGroups] = useState([]);
  const [sortBy, setSortBy] = useState('upload_date');
  const [sortOrder, setSortOrder] = useState('desc');
  const [editingItem, setEditingItem] = useState(null);
  const [categories, setCategories] = useState([]);
  const [subcategories, setSubcategories] = useState([]);
  const { user } = useContext(AuthContext);
  const { groupId } = useParams();
  const navigate = useNavigate();

  useEffect(() => {
    if (user) {
      fetchDocumentGroups();
      fetchCategories();
      fetchSubcategories();
    }
  }, [user, sortBy, sortOrder, groupId]);

  const fetchDocumentGroups = async () => {
    let query = supabase
      .from('document_groups')
      .select(`
        id,
        name,
        created_at,
        document_timestamp,
        extracted_items (
          id,
          description,
          quantity,
          price,
          tax_amount,
          brand,
          category_id,
          subcategory_id,
          user_modified_extracted_items (
            id,
            description,
            quantity,
            price,
            tax_amount,
            brand,
            category_id,
            subcategory_id
          )
        )
      `)
      .eq('user_id', user.id);

    switch (sortBy) {
      case 'name':
        query = query.order('name', { ascending: sortOrder === 'asc' });
        break;
      case 'upload_date':
        query = query.order('created_at', { ascending: sortOrder === 'asc' });
        break;
      case 'slip_date':
        query = query.order('document_timestamp', { ascending: sortOrder === 'asc', nullsFirst: sortOrder === 'asc' });
        break;
      default:
        query = query.order('created_at', { ascending: false });
    }

    if (groupId) {
      query = query.eq('id', groupId);
    }

    const { data, error } = await query;

    if (error) {
      console.error('Error fetching document groups:', error);
      toast({
        title: "Error",
        description: "Failed to fetch items. Please try again.",
        variant: "destructive",
      });
    } else {
      const processedData = data.map(group => ({
        ...group,
        items: group.extracted_items.map(item => ({
          ...item,
          ...item.user_modified_extracted_items[0],
          isModified: item.user_modified_extracted_items.length > 0
        }))
      }));
      setDocumentGroups(processedData);
    }
  };

  const fetchCategories = async () => {
    const { data, error } = await supabase
      .from('categories')
      .select('id, name')
      .eq('user_id', user.id);

    if (error) {
      console.error('Error fetching categories:', error);
    } else {
      setCategories(data);
    }
  };

  const fetchSubcategories = async () => {
    const { data, error } = await supabase
      .from('subcategories')
      .select('id, name, category_id')
      .eq('user_id', user.id);

    if (error) {
      console.error('Error fetching subcategories:', error);
    } else {
      setSubcategories(data);
    }
  };

  const handleEdit = (item, groupId) => {
    setEditingItem({
      ...item,
      document_group_id: groupId
    });
  };

  const handleSave = async () => {
    if (!editingItem) return;
    const { data, error } = await supabase
      .from('user_modified_extracted_items')
      .upsert({
        user_id: user.id,
        document_group_id: editingItem.document_group_id,
        original_extracted_item_id: editingItem.id,
        description: editingItem.description,
        quantity: editingItem.quantity,
        price: editingItem.price,
        tax_amount: editingItem.tax_amount,
        brand: editingItem.brand,
        category_id: editingItem.category_id,
        subcategory_id: editingItem.subcategory_id
      });

    if (error) {
      console.error('Error saving item:', error);
      toast({
        title: "Error",
        description: "Failed to save item. Please try again.",
        variant: "destructive",
      });
    } else {
      toast({
        title: "Success",
        description: "Item saved successfully.",
      });
      setEditingItem(null);
      fetchDocumentGroups();
    }
  };

  const handleDelete = async (item) => {
    // Implement delete logic here
    console.log('Delete item:', item);
    // You'll need to add the actual delete operation with Supabase
    // For example:
    // const { error } = await supabase
    //   .from('user_modified_extracted_items')
    //   .delete()
    //   .match({ id: item.id });
    // 
    // if (error) {
    //   console.error('Error deleting item:', error);
    //   toast({
    //     title: "Error",
    //     description: "Failed to delete item. Please try again.",
    //     variant: "destructive",
    //   });
    // } else {
    //   toast({
    //     title: "Success",
    //     description: "Item deleted successfully.",
    //   });
    //   fetchDocumentGroups();
    // }
  };

  const getCategoryName = (categoryId) => {
    const category = categories.find(c => c.id === categoryId);
    return category ? category.name : 'N/A';
  };

  const getSubcategoryName = (subcategoryId) => {
    const subcategory = subcategories.find(s => s.id === subcategoryId);
    return subcategory ? subcategory.name : 'N/A';
  };

  const renderContent = () => {
    if (documentGroups.length === 0) {
      return (
        <div className="text-center py-8">
          <p className="text-lg mb-4">No items found.</p>
          <p className="text-gray-600 mb-4">To get started, head over to the Slips section and process a group.</p>
          <Button variant="outline" onClick={() => navigate('/slips')}>
            Go to Slips
          </Button>
        </div>
      );
    }

    return (
      <Accordion type="single" collapsible className="w-full">
        {documentGroups.map((group) => (
          <AccordionItem value={group.id} key={group.id}>
            <AccordionTrigger>
              <div className="flex items-center justify-between w-full">
                <div className="flex items-center">
                  <Folder className="mr-2 h-4 w-4" />
                  <span>{group.name || `Group ${group.id}`}</span>
                </div>
                <span className="text-sm text-gray-500 mr-4">
                  {group.document_timestamp ? new Date(group.document_timestamp).toLocaleDateString() : "No Date"}
                </span>
              </div>
            </AccordionTrigger>
            <AccordionContent>
              {group.items.length === 0 ? (
                <div className="text-center py-4">
                  <p className="text-gray-600">No items in this group. Process this group in the Slips section to add items.</p>
                </div>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Description</TableHead>
                      <TableHead>Quantity</TableHead>
                      <TableHead>Price</TableHead>
                      <TableHead>Tax Amount</TableHead>
                      <TableHead>Brand</TableHead>
                      <TableHead>Category</TableHead>
                      <TableHead>Subcategory</TableHead>
                      <TableHead>Actions</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {group.items.map((item) => (
                      <TableRow key={item.id}>
                        <TableCell>
                          {editingItem?.id === item.id ? (
                            <Input
                              value={editingItem.description}
                              onChange={(e) => setEditingItem({...editingItem, description: e.target.value})}
                            />
                          ) : (
                            item.description
                          )}
                        </TableCell>
                        <TableCell>
                          {editingItem?.id === item.id ? (
                            <Input
                              type="number"
                              value={editingItem.quantity}
                              onChange={(e) => setEditingItem({...editingItem, quantity: e.target.value})}
                            />
                          ) : (
                            item.quantity
                          )}
                        </TableCell>
                        <TableCell>
                          {editingItem?.id === item.id ? (
                            <Input
                              type="number"
                              value={editingItem.price}
                              onChange={(e) => setEditingItem({...editingItem, price: e.target.value})}
                            />
                          ) : (
                            item.price
                          )}
                        </TableCell>
                        <TableCell>
                          {editingItem?.id === item.id ? (
                            <Input
                              type="number"
                              value={editingItem.tax_amount}
                              onChange={(e) => setEditingItem({...editingItem, tax_amount: e.target.value})}
                            />
                          ) : (
                            item.tax_amount
                          )}
                        </TableCell>
                        <TableCell>
                          {editingItem?.id === item.id ? (
                            <Input
                              value={editingItem.brand}
                              onChange={(e) => setEditingItem({...editingItem, brand: e.target.value})}
                            />
                          ) : (
                            item.brand
                          )}
                        </TableCell>
                        <TableCell>
                          {editingItem?.id === item.id ? (
                            <Select
                              value={editingItem.category_id}
                              onValueChange={(value) => setEditingItem({...editingItem, category_id: value})}
                            >
                              <SelectTrigger>
                                <SelectValue placeholder="Select category" />
                              </SelectTrigger>
                              <SelectContent>
                                {categories.map((category) => (
                                  <SelectItem key={category.id} value={category.id}>
                                    {category.name}
                                  </SelectItem>
                                ))}
                              </SelectContent>
                            </Select>
                          ) : (
                            getCategoryName(item.category_id)
                          )}
                        </TableCell>
                        <TableCell>
                          {editingItem?.id === item.id ? (
                            <Select
                              value={editingItem.subcategory_id}
                              onValueChange={(value) => setEditingItem({...editingItem, subcategory_id: value})}
                            >
                              <SelectTrigger>
                                <SelectValue placeholder="Select subcategory" />
                              </SelectTrigger>
                              <SelectContent>
                                {subcategories
                                  .filter((subcategory) => subcategory.category_id === editingItem.category_id)
                                  .map((subcategory) => (
                                    <SelectItem key={subcategory.id} value={subcategory.id}>
                                      {subcategory.name}
                                    </SelectItem>
                                  ))}
                              </SelectContent>
                            </Select>
                          ) : (
                            getSubcategoryName(item.subcategory_id)
                          )}
                        </TableCell>
                        <TableCell>
                          <div className="flex space-x-2">
                            {editingItem?.id === item.id ? (
                              <Button 
                                variant="ghost" 
                                size="icon" 
                                onClick={handleSave}
                              >
                                <Save className="h-4 w-4" />
                              </Button>
                            ) : (
                              <>
                                <Button 
                                  variant="ghost" 
                                  size="icon" 
                                  onClick={() => handleEdit(item, group.id)}
                                >
                                  <Edit className="h-4 w-4" />
                                </Button>
                                <Button 
                                  variant="ghost" 
                                  size="icon" 
                                  onClick={() => handleDelete(item)}
                                >
                                  <Trash2 className="h-4 w-4" />
                                </Button>
                              </>
                            )}
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </AccordionContent>
          </AccordionItem>
        ))}
      </Accordion>
    );
  };

  return (
    <Card className="w-full max-w-[1200px] mx-auto">
      <CardHeader>
        <CardTitle className="flex flex-col space-y-4">
          <span className="text-2xl font-bold">Items</span>
          <div className="flex flex-col space-y-2 sm:flex-row sm:space-y-0 sm:items-center">
            <div className="flex items-center space-x-2 w-full sm:w-auto">
              <Select value={sortBy} onValueChange={setSortBy}>
                <SelectTrigger className="w-full sm:w-[180px]">
                  <SelectValue placeholder="Sort by" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="name">Name</SelectItem>
                  <SelectItem value="upload_date">Upload Date</SelectItem>
                  <SelectItem value="slip_date">Slip Date</SelectItem>
                </SelectContent>
              </Select>
              <Button
                variant="outline"
                onClick={() => setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc')}
                className="w-[50px]"
              >
                {sortOrder === 'asc' ? <ArrowUp className="h-4 w-4" /> : <ArrowDown className="h-4 w-4" />}
              </Button>
            </div>
          </div>
        </CardTitle>
      </CardHeader>
      <CardContent>
        {renderContent()}
      </CardContent>
    </Card>
  );
};

export default Items;