import { z } from "zod";

// User schema (from /api/me, /api/login, /api/signup)
export const userSchema = z.object({
  user_id: z.string().uuid(),
  username: z.string(),
  email: z.string().email(),
  created_at: z.string().optional(),
});

export type User = z.infer<typeof userSchema>;

// Login response
export const loginResponseSchema = z.object({
  status: z.string(),
  user_id: z.string().uuid(),
  username: z.string(),
  email: z.string().email(),
  created_at: z.string(),
  expires_at: z.string(),
});

export type LoginResponse = z.infer<typeof loginResponseSchema>;

// Signup response
export const signupResponseSchema = z.object({
  user_id: z.string().uuid(),
  username: z.string(),
  email: z.string().email(),
});

export type SignupResponse = z.infer<typeof signupResponseSchema>;

// Me response (same as user)
export const meResponseSchema = userSchema;
export type MeResponse = z.infer<typeof meResponseSchema>;

// Logout response
export const logoutResponseSchema = z.object({
  status: z.string(),
});

export type LogoutResponse = z.infer<typeof logoutResponseSchema>;

// Error response from backend
export const errorResponseSchema = z.object({
  error: z.string(),
});

export type ErrorResponse = z.infer<typeof errorResponseSchema>;

// Conversation schemas
export const conversationSchema = z.object({
  id: z.string().uuid(),
  type: z.enum(["group", "private"]),
  title: z.string().nullable(),
  display_name: z.string().nullable(),
  created_at: z.string(),
  updated_at: z.string(),
});

export type Conversation = z.infer<typeof conversationSchema>;

export const conversationListResponseSchema = z.object({
  conversations: z.array(conversationSchema),
});

export type ConversationListResponse = z.infer<typeof conversationListResponseSchema>;

// Message schemas
export const messageSchema = z.object({
  id: z.string().uuid(),
  conversation_id: z.string().uuid(),
  sender_id: z.string().uuid(),
  sender_username: z.string(),
  content: z.string(),
  created_at: z.string(),
});

export type Message = z.infer<typeof messageSchema>;

export const messageListResponseSchema = z.object({
  messages: z.array(messageSchema).nullable(),
  next_cursor: z.string().nullable(),
});

export type MessageListResponse = z.infer<typeof messageListResponseSchema>;

// User search
export const userSearchResultSchema = z.object({
  id: z.string().uuid(),
  username: z.string(),
});

export type UserSearchResult = z.infer<typeof userSearchResultSchema>;

export const userSearchResponseSchema = z.object({
  users: z.array(userSearchResultSchema),
});

export type UserSearchResponse = z.infer<typeof userSearchResponseSchema>;
