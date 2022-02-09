// Copyright 2019-present Facebook
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package globalid_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"entgo.io/contrib/entgql"
	gen "entgo.io/contrib/entgql/internal/globalid"
	"entgo.io/contrib/entgql/internal/globalid/ent"
	"entgo.io/contrib/entgql/internal/globalid/ent/enttest"
	"entgo.io/contrib/entgql/internal/globalid/ent/migrate"
	"entgo.io/ent/dialect"
	"github.com/99designs/gqlgen/client"
	"github.com/99designs/gqlgen/graphql/handler"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/suite"
)

type globalidTestSuite struct {
	suite.Suite
	*client.Client
	ent *ent.Client
}

const (
	queryAll = `query {
		users {
			totalCount
			edges {
				node {
					id
					name
				}
				cursor
			}
			pageInfo {
				hasNextPage
				hasPreviousPage
				startCursor
				endCursor
			}
		}
	}`
)

func (s *globalidTestSuite) SetupTest() {
	s.ent = enttest.Open(s.T(), dialect.SQLite,
		fmt.Sprintf("file:%s-%d?mode=memory&cache=shared&_fk=1",
			s.T().Name(), time.Now().UnixNano(),
		),
		enttest.WithMigrateOptions(migrate.WithGlobalUniqueID(true)),
	)

	srv := handler.NewDefaultServer(gen.NewSchema(s.ent))
	srv.Use(entgql.Transactioner{TxOpener: s.ent})
	s.Client = client.New(srv)
}

func TestGlobalID(t *testing.T) {
	suite.Run(t, &globalidTestSuite{})
}

type response struct {
	Users struct {
		TotalCount int
		Edges      []struct {
			Node struct {
				ID   string
				Name string
			}
			Cursor string
		}
		PageInfo struct {
			HasNextPage     bool
			HasPreviousPage bool
			StartCursor     *string
			EndCursor       *string
		}
	}
}

func (s *globalidTestSuite) TestQueryAll() {
	ctx := context.Background()
	s.ent.User.Delete().ExecX(ctx)
	u1 := s.ent.User.Create().SetName("U1").SaveX(ctx)
	u2 := s.ent.User.Create().SetName("U2").SaveX(ctx)
	u3 := s.ent.User.Create().SetName("U3").SaveX(ctx)
	u4 := s.ent.User.Create().SetName("U4").SaveX(ctx)

	var rsp response
	err := s.Post(queryAll, &rsp)
	s.Require().NoError(err)
	s.Require().Equal(4, rsp.Users.TotalCount)
	s.Require().Equal(u1.GlobalID().String(), rsp.Users.Edges[0].Node.ID)
	s.Require().Equal(u2.GlobalID().String(), rsp.Users.Edges[1].Node.ID)
	s.Require().Equal(u3.GlobalID().String(), rsp.Users.Edges[2].Node.ID)
	s.Require().Equal(u4.GlobalID().String(), rsp.Users.Edges[3].Node.ID)
}

func (s *globalidTestSuite) TestPaginationFiltering() {
	ctx := context.Background()
	s.ent.User.Delete().ExecX(ctx)
	u1 := s.ent.User.Create().SetName("U1").SaveX(ctx)
	s.ent.User.Create().SetName("U2").SaveX(ctx)
	s.ent.User.Create().SetName("U3").SaveX(ctx)
	s.ent.User.Create().SetName("U4").SaveX(ctx)

	const (
		query = `query($after: Cursor, $first: Int, $before: Cursor, $last: Int, $id: ID) {
			users(after: $after, first: $first, before: $before, last: $last, where: {id: $id}) {
				totalCount
				edges {
					node {
						id
						name
					}
					cursor
				}
				pageInfo {
					hasNextPage
					hasPreviousPage
					startCursor
					endCursor
				}
			}
		}`
	)
	s.Run("with id", func() {
		var rsp response
		err := s.Post(query, &rsp,
			client.Var("first", 1),
			client.Var("id", u1.GlobalID().String()),
		)
		s.NoError(err)
		s.Require().Equal(1, rsp.Users.TotalCount)
	})
}

func (s *globalidTestSuite) TestNode() {
	ctx := context.Background()
	s.ent.User.Delete().ExecX(ctx)
	u1 := s.ent.User.Create().SetName("U1").SaveX(ctx)
	s.ent.User.Create().SetName("U2").SaveX(ctx)
	s.ent.User.Create().SetName("U3").SaveX(ctx)
	s.ent.User.Create().SetName("U4").SaveX(ctx)

	const (
		query = `query($id: ID!) {
			user: node(id: $id) {
				... on User {
					id
					name
				}
			}
		}`
	)
	var rsp struct {
		User struct {
			ID   string
			Name string
		}
	}
	err := s.Post(query, &rsp, client.Var("id", u1.GlobalID().String()))
	s.Require().NoError(err)
	s.Require().Equal("U1", rsp.User.Name)
}

func (s *globalidTestSuite) TestNodes() {
	ctx := context.Background()
	s.ent.User.Delete().ExecX(ctx)
	u1 := s.ent.User.Create().SetName("U1").SaveX(ctx)
	u2 := s.ent.User.Create().SetName("U2").SaveX(ctx)
	s.ent.User.Create().SetName("U3").SaveX(ctx)
	s.ent.User.Create().SetName("U4").SaveX(ctx)

	const (
		query = `query($ids: [ID!]!) {
			users: nodes(ids: $ids) {
				... on User {
					id
					name
				}
			}
		}`
	)
	var rsp struct {
		Users []struct {
			ID   string
			Name string
		}
	}
	err := s.Post(query, &rsp, client.Var("ids", []string{
		u1.GlobalID().String(),
		u2.GlobalID().String(),
	}))
	s.Require().NoError(err)
	s.Require().Len(rsp.Users, 2)
	s.Require().Equal("U1", rsp.Users[0].Name)
	s.Require().Equal("U2", rsp.Users[1].Name)
}
