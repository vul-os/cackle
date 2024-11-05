import { createContext } from 'react';

export const OrderContext = createContext(null);

export const ACTIONS = {
  SET_ORDER: 'SET_ORDER',
  UPDATE_ORDER: 'UPDATE_ORDER',
  CLEAR_ORDER: 'CLEAR_ORDER',
};

export function orderReducer(state, action) {
  console.log('Order Reducer - Action:', action.type, action.payload);

  switch (action.type) {
    case ACTIONS.SET_ORDER:
      return action.payload ? { ...action.payload } : null;

    case ACTIONS.UPDATE_ORDER:
      return state ? { ...state, ...action.payload } : null;

    case ACTIONS.CLEAR_ORDER:
      return null;

    default:
      return state;
  }
}