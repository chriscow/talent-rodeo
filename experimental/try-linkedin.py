import os
from dotenv import load_dotenv
from linkedin_api import Linkedin
from pprint import pprint

load_dotenv()

user = os.getenv("LINKEDIN_USERNAME")
password = os.getenv("LINKEDIN_PASSWORD")
if not user or not password:
    raise Exception("Please provide a Linkedin username and password")

# Authenticate using any Linkedin user account credentials
api = Linkedin(user, password)

# GET a profile
# profile = api.get_profile("chriscowherd")
# pprint(profile)

# GET a profiles contact info
# contact_info = api.get_profile_contact_info("chriscowherd")
# pprint(contact_info)

# GET 1st degree connections of a given profile
# connections = api.get_profile_connections("1234asc12304")

resp = api.get_conversations()
conversations = resp["elements"]
pprint(conversations)

most_recent_convo = conversations[0]
next_most_recent_convo = conversations[1]

urn_tokens = most_recent_convo["dashEntityUrn"].split(":")
urn = urn_tokens[-1]

most_recent_msg = next_most_recent_convo["events"][0]

# this appears to be the same thing as conversations['elements'][0]
# messages = api.get_conversation(urn)
# pprint(messages)

details = api.get_conversation_details(urn)
pprint(details)


# message:
"""
{'dashEntityUrn':
'urn:li:fsd_conversation:2-NjY3NTliODUtYmFjZS00YzBjLTkwMTItNDNjMTc4ZmZkOWUyXzAxMA==',
'inboxType': 'PRIMARY', 'notificationStatus': 'ACTIVE', 'read': True,
'groupChat': False, 'totalEventCount': 1, 'unreadCount': 0, 'lastActivityAt':
1733704535516, 'firstMessageUrn':
'urn:li:fs_event:(2-NjY3NTliODUtYmFjZS00YzBjLTkwMTItNDNjMTc4ZmZkOWUyXzAxMA==,2-MTczMzcwNDIwOTM0M2I1MDc0Ny0wMDEmNjY3NTliODUtYmFjZS00YzBjLTkwMTItNDNjMTc4ZmZkOWUyXzAxMA==)',
'backendUrn':
'urn:li:messagingThread:2-NjY3NTliODUtYmFjZS00YzBjLTkwMTItNDNjMTc4ZmZkOWUyXzAxMA==',
'receipts': [], 'lastReadAt': 1733704542627, 'archived': False, 'blocked':
False, 'starred': False, 'entityUrn':
'urn:li:fs_conversation:2-NjY3NTliODUtYmFjZS00YzBjLTkwMTItNDNjMTc4ZmZkOWUyXzAxMA==',
'sdkEntityUrn':
'urn:li:msg_conversation:(urn:li:fsd_profile:ACoAAABoCDYBtvdOGaW5dGayS7H0KL1uWNasrNo,2-NjY3NTliODUtYmFjZS00YzBjLTkwMTItNDNjMTc4ZmZkOWUyXzAxMA==)',
'viewerCurrentParticipant': True, 'featureTypes': ['REACTIONS',
'CREATE_NEW_GROUP_CHAT', 'CREATE_LINKEDIN_VIRTUAL_MEETING'],
'withNonConnection': False, 'muted': False, 'events': [{'createdAt':
1733704535506, 'reactionSummaries': [], 'dashEntityUrn':
'urn:li:fsd_message:2-MTczMzcwNDUzNTI0OGI5NDI1Ni0wMDEmNjY3NTliODUtYmFjZS00YzBjLTkwMTItNDNjMTc4ZmZkOWUyXzAxMA==',
'entityUrn':
'urn:li:fs_event:(2-NjY3NTliODUtYmFjZS00YzBjLTkwMTItNDNjMTc4ZmZkOWUyXzAxMA==,2-MTczMzcwNDUzNTI0OGI5NDI1Ni0wMDEmNjY3NTliODUtYmFjZS00YzBjLTkwMTItNDNjMTc4ZmZkOWUyXzAxMA==)',
'eventContent': {'com.linkedin.voyager.messaging.event.MessageEvent':
{'messageBodyRenderFormat': 'DEFAULT', 'body': '', 'attributedBody': {'text':
'Weird 319-504-6541'}}}, 'subtype': 'MEMBER_TO_MEMBER', 'from':
{'com.linkedin.voyager.messaging.MessagingMember': {'miniProfile':
{'memorialized': False, 'lastName': 'Melloy', 'dashEntityUrn':
'urn:li:fsd_profile:ACoAAACn4_MBLWg3Gmlu0gPb42dtSnAcTHLVYfE',
'standardizedPronoun': 'HE_HIM', 'occupation': 'Staff Engineer, Backend / Head
of Data at Universe | Leading High-Impact Data Teams', 'objectUrn':
'urn:li:member:11002867', 'backgroundImage': {'com.linkedin.common.VectorImage':
{'artifacts': [{'width': 800, 'fileIdentifyingUrlPathSegment':
'200_800/profile-displaybackgroundimage-shrink_200_800/0/1731102046002?e=1741219200&v=beta&t=PXk5Afd25cjKNYio3HR3tCF_Znkm5qoNAMWekTLc2AQ',
'expiresAt': 1741219200000, 'height': 200}, {'width': 1400,
'fileIdentifyingUrlPathSegment':
'350_1400/profile-displaybackgroundimage-shrink_350_1400/0/1731102046002?e=1741219200&v=beta&t=a2DS8v4xITZJMyiqHmy6poA0mKVXTBkAg87bn39gJ_w',
'expiresAt': 1741219200000, 'height': 350}], 'rootUrl':
'https://media.licdn.com/dms/image/v2/D5616AQEdb-_D_0N_cw/profile-displaybackgroundimage-shrink_'}},
'picture': {'com.linkedin.common.VectorImage': {'artifacts': [{'width': 100,
'fileIdentifyingUrlPathSegment':
'100_100/profile-displayphoto-shrink_100_100/0/1731524228345?e=1741219200&v=beta&t=fghOHkbAjeKOqdOZGFGE-4O1wLJxBOL3r_KhSiGYhvU',
'expiresAt': 1741219200000, 'height': 100}, {'width': 200,
'fileIdentifyingUrlPathSegment':
'200_200/profile-displayphoto-shrink_200_200/0/1731524228345?e=1741219200&v=beta&t=Rs1eOwUayBuby4vn4hPArr40wQOcpj7KZWRsLVmFuE8',
'expiresAt': 1741219200000, 'height': 200}, {'width': 400,
'fileIdentifyingUrlPathSegment':
'400_400/profile-displayphoto-shrink_400_400/0/1731524228345?e=1741219200&v=beta&t=T4qs4033yNZ19FpUlZEuZfYXWcyWsdi3Vqn1IgnvFAQ',
'expiresAt': 1741219200000, 'height': 400}, {'width': 800,
'fileIdentifyingUrlPathSegment':
'800_800/profile-displayphoto-shrink_800_800/0/1731524228373?e=1741219200&v=beta&t=VOVc4CSJHN9Tg87l8on5fARYSQTmlRB_NAGJ9HEw4Sk',
'expiresAt': 1741219200000, 'height': 800}], 'rootUrl':
'https://media.licdn.com/dms/image/v2/D5603AQFYErbUIikNGw/profile-displayphoto-shrink_'}},
'firstName': 'Jeffrey', 'entityUrn':
'urn:li:fs_miniProfile:ACoAAACn4_MBLWg3Gmlu0gPb42dtSnAcTHLVYfE',
'publicIdentifier': 'jeffreymelloy', 'trackingId': 'v/+HAFhzQW++qhH8S0wOoA=='},
'entityUrn':
'urn:li:fs_messagingMember:(2-NjY3NTliODUtYmFjZS00YzBjLTkwMTItNDNjMTc4ZmZkOWUyXzAxMA==,ACoAAACn4_MBLWg3Gmlu0gPb42dtSnAcTHLVYfE)',
'nameInitials': 'JM'}}, 'previousEventInConversation':
'urn:li:fs_event:(2-NjY3NTliODUtYmFjZS00YzBjLTkwMTItNDNjMTc4ZmZkOWUyXzAxMA==,2-MTczMzcwNDIwOTM0M2I1MDc0Ny0wMDEmNjY3NTliODUtYmFjZS00YzBjLTkwMTItNDNjMTc4ZmZkOWUyXzAxMA==)',
'backendUrn':
'urn:li:messagingMessage:2-MTczMzcwNDUzNTI0OGI5NDI1Ni0wMDEmNjY3NTliODUtYmFjZS00YzBjLTkwMTItNDNjMTc4ZmZkOWUyXzAxMA=='}],
'participants': [{'com.linkedin.voyager.messaging.MessagingMember':
{'miniProfile': {'memorialized': False, 'lastName': 'Melloy', 'dashEntityUrn':
'urn:li:fsd_profile:ACoAAACn4_MBLWg3Gmlu0gPb42dtSnAcTHLVYfE',
'standardizedPronoun': 'HE_HIM', 'occupation': 'Staff Engineer, Backend / Head
of Data at Universe | Leading High-Impact Data Teams', 'objectUrn':
'urn:li:member:11002867', 'backgroundImage': {'com.linkedin.common.VectorImage':
{'artifacts': [{'width': 800, 'fileIdentifyingUrlPathSegment':
'200_800/profile-displaybackgroundimage-shrink_200_800/0/1731102046002?e=1741219200&v=beta&t=PXk5Afd25cjKNYio3HR3tCF_Znkm5qoNAMWekTLc2AQ',
'expiresAt': 1741219200000, 'height': 200}, {'width': 1400,
'fileIdentifyingUrlPathSegment':
'350_1400/profile-displaybackgroundimage-shrink_350_1400/0/1731102046002?e=1741219200&v=beta&t=a2DS8v4xITZJMyiqHmy6poA0mKVXTBkAg87bn39gJ_w',
'expiresAt': 1741219200000, 'height': 350}], 'rootUrl':
'https://media.licdn.com/dms/image/v2/D5616AQEdb-_D_0N_cw/profile-displaybackgroundimage-shrink_'}},
'picture': {'com.linkedin.common.VectorImage': {'artifacts': [{'width': 100,
'fileIdentifyingUrlPathSegment':
'100_100/profile-displayphoto-shrink_100_100/0/1731524228345?e=1741219200&v=beta&t=fghOHkbAjeKOqdOZGFGE-4O1wLJxBOL3r_KhSiGYhvU',
'expiresAt': 1741219200000, 'height': 100}, {'width': 200,
'fileIdentifyingUrlPathSegment':
'200_200/profile-displayphoto-shrink_200_200/0/1731524228345?e=1741219200&v=beta&t=Rs1eOwUayBuby4vn4hPArr40wQOcpj7KZWRsLVmFuE8',
'expiresAt': 1741219200000, 'height': 200}, {'width': 400,
'fileIdentifyingUrlPathSegment':
'400_400/profile-displayphoto-shrink_400_400/0/1731524228345?e=1741219200&v=beta&t=T4qs4033yNZ19FpUlZEuZfYXWcyWsdi3Vqn1IgnvFAQ',
'expiresAt': 1741219200000, 'height': 400}, {'width': 800,
'fileIdentifyingUrlPathSegment':
'800_800/profile-displayphoto-shrink_800_800/0/1731524228373?e=1741219200&v=beta&t=VOVc4CSJHN9Tg87l8on5fARYSQTmlRB_NAGJ9HEw4Sk',
'expiresAt': 1741219200000, 'height': 800}], 'rootUrl':
'https://media.licdn.com/dms/image/v2/D5603AQFYErbUIikNGw/profile-displayphoto-shrink_'}},
'firstName': 'Jeffrey', 'entityUrn':
'urn:li:fs_miniProfile:ACoAAACn4_MBLWg3Gmlu0gPb42dtSnAcTHLVYfE',
'publicIdentifier': 'jeffreymelloy', 'trackingId': 'jOKBh8ewTuK4NmiwL2GQJQ=='},
'entityUrn':
'urn:li:fs_messagingMember:(2-NjY3NTliODUtYmFjZS00YzBjLTkwMTItNDNjMTc4ZmZkOWUyXzAxMA==,ACoAAACn4_MBLWg3Gmlu0gPb42dtSnAcTHLVYfE)',
'nameInitials': 'JM'}}]}"
"""
