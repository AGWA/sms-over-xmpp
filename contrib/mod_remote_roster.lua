-- Copyright (c) 2009-2015 Various Contributors (see individual files and source control)
--
-- Permission is hereby granted, free of charge, to any person obtaining a copy of
-- this software and associated documentation files (the "Software"), to deal in
-- the Software without restriction, including without limitation the rights to
-- use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
-- the Software, and to permit persons to whom the Software is furnished to do so,
-- subject to the following conditions:
--
-- The above copyright notice and this permission notice shall be included in all
-- copies or substantial portions of the Software.
--
-- THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
-- IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
-- FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
-- COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
-- IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
-- CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
--
--
-- mod_remote_roster
--
-- This is an experimental implementation of http://jkaluza.fedorapeople.org/remote-roster.html
--
-- Originally written in 2011 by Waqas Hussain and included in prosody-modules.  Patched in
-- 2022 by Andrew Ayer to work with the latest version of Prosody.
--

local st = require "util.stanza";
local jid_split = require "util.jid".split;
local jid_prep = require "util.jid".prep;
local t_concat = table.concat;
local tonumber = tonumber;
local pairs, ipairs = pairs, ipairs;
local hosts = hosts;

local load_roster = require "core.rostermanager".load_roster;
local save_roster = require "core.rostermanager".save_roster;
local rm_remove_from_roster = require "core.rostermanager".remove_from_roster;
local rm_add_to_roster = require "core.rostermanager".add_to_roster;
local rm_roster_push = require "core.rostermanager".roster_push;
local user_exists = require "core.usermanager".user_exists;
local add_task = require "util.timer".add_task;
local new_id = require("util.id").short;

module:hook("iq-get/bare/jabber:iq:roster:query", function(event)
	local origin, stanza = event.origin, event.stanza;

	if origin.type == "component" and stanza.attr.from == origin.host then
		local node, host = jid_split(stanza.attr.to);
		local roster = load_roster(node, host);

		local reply = st.reply(stanza):query("jabber:iq:roster");
		for jid, item in pairs(roster) do
			if jid ~= "pending" and jid then
				local node, host = jid_split(jid);
				if host == origin.host then -- only include contacts which are on this component
					reply:tag("item", {
						jid = jid,
						subscription = item.subscription,
						ask = item.ask,
						name = item.name,
					});
					for group in pairs(item.groups) do
						reply:tag("group"):text(group):up();
					end
					reply:up(); -- move out from item
				end
			end
		end
		origin.send(reply);
		--origin.interested = true; -- resource is interested in roster updates
		return true;
	end
end);

module:hook("iq-set/bare/jabber:iq:roster:query", function(event)
	local session, stanza = event.origin, event.stanza;

	if not(session.type == "component" and stanza.attr.from == session.host) then return; end
	local from_node, from_host = jid_split(stanza.attr.to);
	if not(user_exists(from_node, from_host)) then return; end
	local roster = load_roster(from_node, from_host);
	if not(roster) then return; end

	local query = stanza.tags[1];
	if #query.tags == 1 and query.tags[1].name == "item"
			and query.tags[1].attr.xmlns == "jabber:iq:roster" and query.tags[1].attr.jid
			-- Protection against overwriting roster.pending, until we move it
			and query.tags[1].attr.jid ~= "pending" then
		local item = query.tags[1];
		local jid = jid_prep(item.attr.jid);
		local node, host, resource = jid_split(jid);
		if not resource and host == session.host then
			if jid ~= stanza.attr.to then -- not self-jid
				if item.attr.subscription == "remove" then
					local r_item = roster[jid];
					if r_item then
						local to_bare = node and (node.."@"..host) or host; -- bare JID
						--if r_item.subscription == "both" or r_item.subscription == "from" or (roster.pending and roster.pending[jid]) then
						--	module:send(st.presence({type="unsubscribed", from=stanza.attr.to, to=to_bare}));
						--end
						--if r_item.subscription == "both" or r_item.subscription == "to" or r_item.ask then
						--	module:send(st.presence({type="unsubscribe", from=stanza.attr.to, to=to_bare}));
						--end
						roster[jid] = nil;
						if save_roster(from_node, from_host, roster) then
							session.send(st.reply(stanza));
							rm_roster_push(from_node, from_host, jid);
						else
							roster[jid] = item;
							session.send(st.error_reply(stanza, "wait", "internal-server-error", "Unable to save roster"));
						end
					else
						session.send(st.error_reply(stanza, "modify", "item-not-found"));
					end
				else
					local subscription = item.attr.subscription;
					if subscription ~= "both" and subscription ~= "to" and subscription ~= "from" and subscription ~= "none" then -- TODO error on invalid
						subscription = roster[jid] and roster[jid].subscription or "none";
					end
					local r_item = {name = item.attr.name, groups = {}};
					if r_item.name == "" then r_item.name = nil; end
					r_item.subscription = subscription;
					if subscription ~= "both" and subscription ~= "to" then
						r_item.ask = roster[jid] and roster[jid].ask;
					end
					for _, child in ipairs(item) do
						if child.name == "group" then
							local text = t_concat(child);
							if text and text ~= "" then
								r_item.groups[text] = true;
							end
						end
					end
					local olditem = roster[jid];
					roster[jid] = r_item;
					if save_roster(from_node, from_host, roster) then -- Ok, send success
						session.send(st.reply(stanza));
						-- and push change to all resources
						rm_roster_push(from_node, from_host, jid);
					else -- Adding to roster failed
						roster[jid] = olditem;
						session.send(st.error_reply(stanza, "wait", "internal-server-error", "Unable to save roster"));
					end
				end
			else -- Trying to add self to roster
				session.send(st.error_reply(stanza, "cancel", "not-allowed"));
			end
		else -- Invalid JID added to roster
			session.send(st.error_reply(stanza, "modify", "bad-request")); -- FIXME what's the correct error?
		end
	else -- Roster set didn't include a single item, or its name wasn't  'item'
		session.send(st.error_reply(stanza, "modify", "bad-request"));
	end
	return true;
end);

function component_roster_push(node, host, jid)
	local roster = load_roster(node, host);
	if roster then
		local item = roster[jid];
		local contact_node, contact_host = jid_split(jid);
		local iq_id = new_id();
		local stanza = st.iq({ type="set", from=node.."@"..host, to=contact_host, id = iq_id }):query("jabber:iq:roster");
		if item then
			stanza:tag("item", { jid = jid, subscription = item.subscription, name = item.name, ask = item.ask });
			for group in pairs(item.groups) do
				stanza:tag("group"):text(group):up();
			end
		else
			stanza:tag("item", {jid = jid, subscription = "remove"});
		end
		stanza:up(); -- move out from item
		stanza:up(); -- move out from stanza
		module:send(stanza);
	end
end

module:hook("iq-set/bare/jabber:iq:roster:query", function(event)
	local origin, stanza = event.origin, event.stanza;
	local query = stanza.tags[1];
	local item = query.tags[1];
	local contact_jid = item and item.name == "item" and item.attr.jid ~= "pending" and item.attr.jid;
	if contact_jid then
		local contact_node, contact_host = jid_split(contact_jid);
		if hosts[contact_host] and hosts[contact_host].type == "component" then
			local node, host = jid_split(stanza.attr.to or origin.full_jid);
			add_task(0, function()
				component_roster_push(node, host, contact_jid);
			end);
		end
	end
end, 100);
